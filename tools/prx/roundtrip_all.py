#!/usr/bin/env python3
# Round-trip harness: for each .qb in each prx, decompile then recompile and
# compare bytes. Reports OK / FAIL (byte mismatch) / ERR (tool failed/timeout).
# Usage: roundtrip_all.py <ns-binary> <prx> [<prx> ...]
import sys, os, subprocess, tempfile
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import prx, lzss

NS = os.path.abspath(sys.argv[1])
TIMEOUT = 30

def run(args):
    return subprocess.run(args, capture_output=True, timeout=TIMEOUT)

tot = {'OK':0,'FAIL':0,'ERR':0}
fails=[]; errs=[]
for prxpath in sys.argv[2:]:
    ver, entries = prx.parse(open(prxpath,'rb').read())
    base = os.path.basename(prxpath)
    for e in entries:
        # name field can carry junk after a NUL terminator; the real name is
        # the part before the first NUL (see prx.find).
        name = e['name'].split(b'\0', 1)[0].decode('latin1')
        if not name.lower().endswith('.qb'):
            continue
        orig = lzss.decompress(e['blob'][:e['csize']], e['dsize']) if e['csize'] else e['blob'][:e['dsize']]
        with tempfile.TemporaryDirectory() as d:
            qb=os.path.join(d,'o.qb'); ns=os.path.join(d,'o.ns'); qb2=os.path.join(d,'r.qb')
            open(qb,'wb').write(orig)
            try:
                r=run([NS,'-d',qb,'-o',ns])
                if r.returncode!=0 or not os.path.exists(ns):
                    tot['ERR']+=1; errs.append((base,name,'decompile')); continue
                r=run([NS,'-c',ns,'-o',qb2])
                if r.returncode!=0 or not os.path.exists(qb2):
                    tot['ERR']+=1; errs.append((base,name,'recompile')); continue
            except subprocess.TimeoutExpired:
                tot['ERR']+=1; errs.append((base,name,'timeout')); continue
            rt=open(qb2,'rb').read()
            if rt==orig:
                tot['OK']+=1
            else:
                tot['FAIL']+=1; fails.append((base,name,'%+d'%(len(rt)-len(orig))))

print("\n=== RESULT ===  OK=%d  FAIL=%d  ERR=%d" % (tot['OK'],tot['FAIL'],tot['ERR']))
if fails:
    print("\nFAIL (byte mismatch):")
    for b,n,d in fails: print("  %-20s %-30s %s" % (b,n,d))
if errs:
    print("\nERR (tool failed):")
    for b,n,w in errs: print("  %-20s %-30s %s" % (b,n,w))
