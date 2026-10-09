#!/usr/bin/env python3
"""Inspect actual DEB archives on macOS or Linux without dpkg."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import struct
import tarfile

parser=argparse.ArgumentParser();parser.add_argument('packages',type=Path,nargs='+');args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
version=(root/'VERSION').read_text().strip()
results=[]
for file in args.packages:
    raw=file.read_bytes();assert raw[:8]==b'!<arch>\n','invalid ar'
    offset=8;parts={}
    while offset<len(raw):
        header=raw[offset:offset+60];size=int(header[48:58]);name=header[:16].decode().strip().rstrip('/')
        assert header[58:60]==b'`\n'
        parts[name]=raw[offset+60:offset+60+size];offset+=60+size+size%2
    assert parts['debian-binary']==b'2.0\n'
    def tar(prefix):
        return tarfile.open(fileobj=io.BytesIO(next(v for k,v in parts.items() if k.startswith(prefix))),mode='r:*')
    with tar('control.tar') as control, tar('data.tar') as data:
        def get(archive,path): return archive.extractfile('./'+path).read()
        metadata=get(control,'control').decode()
        arch=re.search(r'^Architecture: (\w+)$',metadata,re.M)[1]
        assert arch in ['amd64','arm64'] and f'Version: {version}\n' in metadata
        assert 'systemd (>= 247)' in metadata and 'init-system-helpers' in metadata
        assert get(control,'conffiles').decode().strip()=='/etc/clash-manager/clash-manager.conf'
        for entry in list(control.getmembers())+list(data.getmembers()):
            assert '..' not in Path(entry.name).parts and not entry.name.startswith('/')
            assert not any(x.startswith('._') for x in Path(entry.name).parts)
            assert entry.uid==entry.gid==0 and entry.mode & 0o022==0, entry.name
        for script in ['postinst','prerm','postrm']:
            assert control.getmember('./'+script).mode==0o755
        for component in ['web','helper']:
            binary=get(data,f'usr/lib/clash-manager/bin/clash-{component}')
            assert binary[:5]==b'\x7fELF\x02'
            assert struct.unpack('<H',binary[18:20])[0]=={'amd64':62,'arm64':183}[arch]
        coremeta=json.loads(get(data,'usr/lib/clash-manager/core/bundled-core.json'))
        corepath=next(m.name for m in data.getmembers() if m.name.endswith('.gz') and 'mihomo-linux-' in m.name)
        core=data.extractfile(corepath).read()
        sourcearch='x86' if arch=='amd64' else 'arm'
        assert core==(root/'resources/core'/sourcearch/Path(corepath).name).read_bytes()
        assert hashlib.sha256(core).hexdigest()==coremeta['sha256']
        assert len(core)==coremeta['size']
        core_elf=gzip.decompress(core)
        assert core_elf[:5]==b'\x7fELF\x02'
        assert struct.unpack('<H',core_elf[18:20])[0]=={'amd64':62,'arm64':183}[arch]
        assert get(data,'etc/clash-manager/clash-manager.conf').decode().startswith('# systemd environment file')
        assert 'LISTEN_ADDR=127.0.0.1:8080' in get(data,'etc/clash-manager/clash-manager.conf').decode()
        assert not any(m.name.endswith('/password') for m in data.getmembers()),'bundled credential'
        for m in data.getmembers():
            if m.name.startswith('./usr/lib/clash-manager/public/') and m.isfile():
                rel=m.name.removeprefix('./usr/lib/clash-manager/public/')
                assert data.extractfile(m).read()==(root/'web/dist'/rel).read_bytes(),rel
        assert 'APP_PLATFORM=linux' in get(data,'lib/systemd/system/clash-manager.service').decode()
        assert 'PartOf=clash-manager.service' in get(data,'lib/systemd/system/clash-manager-helper.service').decode()
        assert b'APP_AUTH_PASSWORD_FILE' in get(data,'usr/lib/clash-manager/start-helper')
    sha=hashlib.sha256(raw).hexdigest()
    assert file.with_suffix('.deb.sha256').read_text().split()[0]==sha
    results.append({'file':file.name,'version':version,'architecture':arch,'sha256':sha,'core':coremeta})
print(json.dumps(results,ensure_ascii=False,indent=2))
