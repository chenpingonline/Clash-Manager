#!/usr/bin/env python3
"""Exercise an isolated Bridge deployment; never modifies the host network."""
import argparse
import http.client
import http.cookiejar
import json
import os
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid

parser = argparse.ArgumentParser()
parser.add_argument('image', nargs='?', default='clash-manager:local')
parser.add_argument('--tun', action='store_true', help='test TUN in an isolated container network with NET_ADMIN')
parser.add_argument('--keep', action='store_true', help='keep the container and volume for local UI QA')
parser.add_argument('--controller-port', type=int, help='exercise Docker startup Controller override')
parser.add_argument('--mixed-port', type=int, help='exercise Docker startup Mixed override')
args = parser.parse_args()
name = 'clash-docker-qa-' + uuid.uuid4().hex[:10]
volume = name + '-data'
# Exercise the minimum accepted password through startup and HTTP login.
password = 'qa-pass8'
envfile = tempfile.NamedTemporaryFile(mode='w', prefix='clash-qa-', delete=False)
envfile.write('APP_AUTH_PASSWORD=' + password + '\n')
if args.controller_port is not None:
    envfile.write(f'APP_CONTROLLER_PORT={args.controller_port}\n')
if args.mixed_port is not None:
    envfile.write(f'APP_MIXED_PORT={args.mixed_port}\n')
envfile.close()
mixed_port = args.mixed_port or 7890

def docker(*argv):
    return subprocess.check_output(['docker', *argv], text=True).strip()

jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
base = ''

def request(path, method='GET', body=None, status=200, authenticated=True):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, method=method, headers={'Content-Type': 'application/json'})
    client = opener if authenticated else urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        response = client.open(req, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        assert response.status == status, f'{path}: HTTP {response.status}, expected {status}'
        return json.loads(raw) if raw else {}

def start():
    global base
    extra = ['--cap-add', 'NET_ADMIN', '--device', '/dev/net/tun:/dev/net/tun'] if args.tun else []
    docker('run', '-d', '--init', '--name', name, '--stop-timeout', '40', '--env-file', envfile.name,
           '-p', '127.0.0.1::8080', '-p', f'127.0.0.1::{mixed_port}', '-v', volume + ':/data', *extra, args.image)
    port = docker('port', name, '8080/tcp').rsplit(':', 1)[1]
    base = 'http://127.0.0.1:' + port
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        try:
            request('/api/health', authenticated=False)
            return
        except (OSError, AssertionError):
            time.sleep(0.5)
    raise RuntimeError('Web did not start')

try:
    docker('volume', 'create', volume)
    start()
    docker('exec', name, '/opt/clash/healthcheck.sh')
    request('/api/settings', status=401, authenticated=False)
    request('/api/auth/login', 'POST', {'username': 'admin', 'password': 'wrong-password'}, status=401)
    request('/api/auth/login', 'POST', {'username': 'admin', 'password': password})
    runtime = request('/api/runtime')
    assert runtime['platform'] == 'docker'
    assert not runtime['capabilities']['hostProxyEnvironment']
    request('/api/system/proxy-environment', 'PUT', {'enabled': True}, status=409)
    request('/api/app/icon', 'PUT', {'id': 'cat-orbit'}, status=409)
    request('/api/settings', 'PUT', {'persistSelections': False})
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        core = request('/api/system/status')
        if core.get('canRestartService'):
            break
        time.sleep(0.5)
    else:
        raise RuntimeError('Bundled Mihomo did not start')
    network = request('/api/network/settings')
    assert network['tunCapability']['supported'] is args.tun
    assert network['settings']['tun']['enabled'] is False
    if args.controller_port is not None:
        assert network['settings']['controller']['port'] == args.controller_port
        assert request('/api/settings')['controller'] == f'http://127.0.0.1:{args.controller_port}'
    assert network['settings']['mixed']['port'] == mixed_port
    proxy_port = int(docker('port', name, f'{mixed_port}/tcp').rsplit(':', 1)[1])
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            conn = http.client.HTTPConnection('127.0.0.1', proxy_port, timeout=10)
            conn.request('GET', 'http://127.0.0.1:8080/api/health')
            response = conn.getresponse()
            raw = response.read()
            if response.status == 200 and json.loads(raw)['ok'] is True:
                break
        except (OSError, ValueError):
            pass
        finally:
            conn.close()
        time.sleep(0.5)
    else:
        raise RuntimeError('Mixed proxy did not become ready')
    if args.tun:
        def routes():
            return sorted(json.loads(docker('exec', name, 'ip', '-j', 'route', 'show', 'table', 'all')), key=lambda item: json.dumps(item, sort_keys=True))
        original_routes = routes()
        result = request('/api/network/tun', 'PUT', {'enabled': True})
        assert result['enabled'] is True
        links = json.loads(docker('exec', name, 'ip', '-j', '-d', 'link', 'show'))
        assert any(item.get('linkinfo', {}).get('info_kind') == 'tun' for item in links), 'TUN device missing'
        assert routes() != original_routes, 'TUN did not install routes'
        result = request('/api/network/tun', 'PUT', {'enabled': False})
        assert result['enabled'] is False
        assert routes() == original_routes, 'routes not restored after TUN disable'
        print('PASS: isolated TUN enable, route installation, disable and route restoration')
    # Persist different ports through the UI API, then prove startup env wins
    # when recreating against the same existing data volume.
    changes = {}
    if args.controller_port is not None and args.controller_port != 9090:
        changes['controller'] = {'enabled': True, 'port': 9090}
    if args.mixed_port is not None and args.mixed_port != 7890:
        changes['mixed'] = {'enabled': True, 'port': 7890}
    if changes:
        request('/api/network/settings', 'PUT', changes)
    docker('stop', name)
    assert docker('inspect', '-f', '{{.State.ExitCode}}', name) == '0', 'unclean container stop'
    docker('rm', name)
    start()
    request('/api/auth/login', 'POST', {'username': 'admin', 'password': password})
    assert request('/api/settings')['persistSelections'] is False, 'lost persisted settings'
    network = request('/api/network/settings')
    if args.controller_port is not None:
        assert network['settings']['controller']['port'] == args.controller_port, 'startup did not override saved Controller'
    if args.mixed_port is not None:
        assert network['settings']['mixed']['port'] == args.mixed_port, 'startup did not override saved Mixed port'
    # Every recreation must leave the Web process unprivileged.
    uid = docker('exec', name, 'sh', '-c', "for path in /proc/[0-9]*; do if [ \"$(cat $path/comm 2>/dev/null)\" = clash-web ]; then awk '/^Uid:/{print $2}' $path/status; fi; done")
    assert uid == '10001', 'Web did not drop root'
    request('/api/auth/logout', 'POST')
    request('/api/settings', status=401)
    print('PASS: authentication, native-feature guards, bundled Core, proxy, persistence, privilege separation, graceful stop')
    if args.keep:
        print('UI QA: ' + base)
        print('Container: ' + name + '; volume: ' + volume)
except Exception:
    try:
        print(docker('logs', '--tail', '40', name))
    except subprocess.CalledProcessError:
        pass
    raise
finally:
    os.unlink(envfile.name)
    if not args.keep:
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(['docker', 'volume', 'rm', volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
