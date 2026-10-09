#!/usr/bin/env python3
"""Install and validate a DEB in an isolated systemd container (never host networking)."""
import argparse
from pathlib import Path
import subprocess
import time
import uuid

parser=argparse.ArgumentParser()
parser.add_argument('package',type=Path)
parser.add_argument('--image',default='clash-manager:deb-test-debian12')
parser.add_argument('--platform',choices=['linux/amd64','linux/arm64'])
parser.add_argument('--keep',action='store_true')
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
name='clash-deb-qa-'+uuid.uuid4().hex[:10]
def docker(*args):
    return subprocess.check_output(['docker',*args],text=True).strip()
try:
    extra=['--platform',args.platform] if args.platform else []
    docker('run','-d','--name',name,'--privileged','--cgroupns=private','--tmpfs','/run','--tmpfs','/tmp','-p','127.0.0.1::8080',*extra,args.image)
    end=time.monotonic()+60
    while time.monotonic()<end:
        try:
            docker('exec',name,'systemctl','show-environment');break
        except subprocess.CalledProcessError:
            time.sleep(.5)
    else: raise RuntimeError('systemd did not boot')
    # Container distributions ship a policy blocking automatic service startup.
    docker('exec',name,'rm','-f','/usr/sbin/policy-rc.d')
    docker('cp',str(args.package.resolve()),name+':/package.deb')
    docker('cp',str(root/'packaging/deb/smoke.py'),name+':/smoke.py')
    extra=['-e','KEEP_DEB_QA=1'] if args.keep else []
    print(docker('exec',*extra,name,'python3','/smoke.py'))
    if args.keep:
        print('Container:',name)
        print('UI: http://'+docker('port',name,'8080/tcp'))
        print('QA login: admin / qa-pass8')
except Exception:
    try:
        print(docker('exec',name,'journalctl','-u','clash-manager','-u','clash-manager-helper','--no-pager','-n','30'))
    except subprocess.CalledProcessError: pass
    raise
finally:
    if not args.keep:
        docker('rm','-f',name)
