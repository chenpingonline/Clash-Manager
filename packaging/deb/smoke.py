#!/usr/bin/env python3
"""Run inside an isolated systemd container after copying the DEB to /package.deb."""
import hashlib
import http.client
import http.cookiejar
import http.server
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()

def wait(fn, timeout=60):
    end = time.monotonic() + timeout
    while True:
        try:
            return fn()
        except (OSError, AssertionError, subprocess.CalledProcessError):
            if time.monotonic() >= end:
                raise
            time.sleep(.3)

def ensure(condition, message):
    assert condition, message

jar = http.cookiejar.CookieJar()
client = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
def request(path, method='GET', body=None, status=200):
    req = urllib.request.Request('http://127.0.0.1:8080'+path, data=None if body is None else json.dumps(body).encode(), method=method, headers={'Content-Type':'application/json'})
    try:
        resp = client.open(req, timeout=10)
    except urllib.error.HTTPError as exc:
        resp = exc
    with resp:
        raw=resp.read()
        assert resp.status==status, f'{path}: {resp.status}: {raw[:300]!r}'
        return json.loads(raw)

def login():
    request('/api/auth/login','POST',{'username':'admin','password':Path('/etc/clash-manager/password').read_text().strip()})

def free(port):
    with socket.socket() as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind(('127.0.0.1',port))

def no_core():
    for file in Path('/proc').glob('[0-9]*/comm'):
        try:
            ensure(file.read_text().strip() != 'mihomo', 'Core process survived')
        except FileNotFoundError:
            pass

def no_tun():
    links=json.loads(run('ip','-j','-d','link'))
    ensure(not any(x.get('linkinfo',{}).get('info_kind')=='tun' for x in links), 'TUN survived service shutdown')

run('dpkg','-i','/package.deb')
# Container base images deliberately suppress service starts via policy-rc.d.
ensure(run('systemctl','is-active','clash-manager')=='active','postinst did not start service')
run('systemd-analyze','verify','/lib/systemd/system/clash-manager.service','/lib/systemd/system/clash-manager-helper.service')
wait(lambda: request('/api/health'))
request('/api/runtime',status=401)
login()
runtime=request('/api/runtime')
ensure(runtime['platform']=='linux' and runtime['displayName']=='Clash Manager', 'runtime branding')
ensure(not any(runtime['capabilities'].values()), 'fnOS capabilities leaked')
request('/api/system/proxy-environment','PUT',{},409)
ensure(request('/api/app/update-info')['delivery']=='deb', 'package upgrade delivery')
wait(lambda: ensure(request('/api/status').get('online') is True, 'Core not online'))
net=wait(lambda: request('/api/network/settings'))
ensure(not net['settings']['tun']['enabled'], 'fresh installation enabled TUN')
webpid=run('systemctl','show','-p','MainPID','--value','clash-manager')
proc=Path(f'/proc/{webpid}/status').read_text()
ensure('Uid:\t0\t' not in proc and 'CapEff:\t0000000000000000' in proc, 'Web privileges')
ensure(run('stat','-c','%a:%U:%G','/etc/clash-manager/password')=='640:root:clash-manager', 'credential permissions')
ensure(run('stat','-c','%a:%U:%G','/run/clash-manager/helper.sock')=='660:root:clash-manager', 'socket permissions')
ensure(run('stat','-c','%a:%U:%G','/var/lib/clash-manager/state')=='1770:root:clash-manager','managed binary parent protection')
blocked=subprocess.run(['runuser','-u','clash-manager','--','mv','/var/lib/clash-manager/state/managed-core','/var/lib/clash-manager/state/replaced-core'],capture_output=True)
ensure(blocked.returncode!=0,'Web could replace privileged Core directory')

# A real HTTP destination proves Mixed proxy forwarding without external network access.
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200);self.end_headers();self.wfile.write(b'deb-proxy-ok')
    def log_message(self,*args): pass
server=http.server.HTTPServer(('127.0.0.1',0),Handler)
threading.Thread(target=server.serve_forever,daemon=True).start()
def proxy():
    conn=http.client.HTTPConnection('127.0.0.1',7890,timeout=10)
    conn.request('GET',f'http://127.0.0.1:{server.server_port}/')
    resp=conn.getresponse();ensure(resp.read()==b'deb-proxy-ok','proxy forwarding');conn.close()
wait(proxy)
server.shutdown()
request('/api/settings','PUT',{'persistSelections':False})
credential_hash=hashlib.sha256(Path('/etc/clash-manager/password').read_bytes()).hexdigest()
conf=Path('/etc/clash-manager/clash-manager.conf')
conf.write_text(conf.read_text().replace('LISTEN_ADDR=127.0.0.1:8080','LISTEN_ADDR=0.0.0.0:8080')+'\nAPP_CONTROLLER_PORT=19097\nAPP_MIXED_PORT=17897\n')
run('systemctl','restart','clash-manager')
wait(lambda: request('/api/health'));login()
wait(lambda: ensure(request('/api/status').get('online') is True,'Core restart'))
net=request('/api/network/settings')
ensure(net['settings']['controller']['port']==19097 and net['settings']['mixed']['port']==17897, 'startup port overrides')
ensure(request('/api/settings')['persistSelections'] is False,'settings persistence')

# Test the actual dpkg upgrade script sequence using a newer metadata-only fixture.
work=Path(tempfile.mkdtemp())
run('dpkg-deb','-R','/package.deb',str(work))
control=work/'DEBIAN/control'
control.write_text(control.read_text().replace('Version: '+runtime['version'], 'Version: '+runtime['version']+'+qa1'))
run('dpkg-deb','--root-owner-group','-Zxz','-b',str(work),'/tmp/upgrade.deb')
shutil.rmtree(work)
run('dpkg','-i','/tmp/upgrade.deb')
ensure(run('systemctl','is-active','clash-manager')=='active','upgrade did not restart service')
wait(lambda: request('/api/health'));login()
ensure(hashlib.sha256(Path('/etc/clash-manager/password').read_bytes()).hexdigest()==credential_hash,'upgrade reset password')
ensure('LISTEN_ADDR=0.0.0.0:8080' in conf.read_text(),'upgrade reset conffile')
ensure(request('/api/settings')['persistSelections'] is False,'upgrade reset settings')
wait(lambda: ensure(request('/api/status').get('online') is True,'upgrade Core start'))

# Host-mode Linux runs in this container's own network namespace, never the real host.
if Path('/dev/net/tun').exists():
    before=run('ip','-j','rule','show')
    result=request('/api/network/tun','PUT',{'enabled':True})
    ensure(result.get('applied') is not False,'TUN enable')
    wait(lambda: ensure(any(x.get('linkinfo',{}).get('info_kind')=='tun' for x in json.loads(run('ip','-j','-d','link'))),'TUN missing'))
    run('systemctl','stop','clash-manager')
    wait(no_tun);wait(no_core)
    ensure(run('ip','-j','rule','show')==before,'policy rules not restored')
    free(8080);free(17897);free(19097)
    run('systemctl','start','clash-manager');wait(lambda: request('/api/health'));login()
    wait(lambda: ensure(request('/api/status').get('online') is True,'restart after TUN'))
    request('/api/network/tun','PUT',{'enabled':False});wait(no_tun)
    print('PASS: isolated TUN service-stop cleanup, routes and ports')
else:
    print('SKIP: TUN device unavailable')
# Invalid credentials must not let Helper restore a previously enabled TUN.
secret=Path('/etc/clash-manager/password')
saved=secret.read_bytes()
run('systemctl','stop','clash-manager')
secret.write_text('short\n')
failed=subprocess.run(['systemctl','start','clash-manager'],capture_output=True)
ensure(failed.returncode!=0,'invalid password started service')
run('systemctl','stop','clash-manager-helper')
wait(no_core);wait(no_tun)
secret.write_bytes(saved)
run('systemctl','reset-failed','clash-manager','clash-manager-helper')
run('systemctl','start','clash-manager');wait(lambda: request('/api/health'))
print('PASS: install, auth, managed Core, proxy, Web permissions, port overrides, restart, upgrade persistence and credential guard')
if os.environ.get('KEEP_DEB_QA')=='1':
    Path('/etc/clash-manager/password').write_text('qa-pass8\n')
    run('systemctl','restart','clash-manager')
    wait(lambda: request('/api/health'))
    raise SystemExit(0)
run('dpkg','--remove','clash-manager')
wait(no_core);wait(no_tun)
free(8080);free(17897);free(19097)
ensure(conf.exists() and Path('/etc/clash-manager/password').exists(),'remove erased configuration')
ensure(Path('/var/lib/clash-manager/config/settings.json').exists(),'remove erased data')
run('dpkg','--purge','clash-manager')
ensure(not conf.exists() and not Path('/etc/clash-manager/password').exists(),'purge retained package credentials')
ensure(Path('/var/lib/clash-manager/config/settings.json').exists(),'purge erased user data')
print('PASS: removal stops both services, purge removes credentials and preserves user data')
