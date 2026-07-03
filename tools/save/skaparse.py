import struct, sys
d=open('/tmp/cas.ska','rb').read()

# ESYMBOLTYPE (THUG2 CStruct)
T={0:'NONE',1:'INT',2:'FLOAT',3:'STRING',4:'LSTRING',5:'PAIR',6:'VECTOR',
   7:'QSCRIPT',8:'CFUNC',9:'MFUNC',10:'STRUCT',11:'STRUCTPTR',12:'ARRAY',13:'NAME'}

class P:
    def __init__(s,d,o): s.d=d; s.o=o
    def u8(s): v=s.d[s.o]; s.o+=1; return v
    def u32(s): v=struct.unpack_from('<I',s.d,s.o)[0]; s.o+=4; return v
    def f32(s): v=struct.unpack_from('<f',s.d,s.o)[0]; s.o+=4; return v
    def cstr(s):
        e=s.d.index(b'\0',s.o); v=s.d[s.o:e]; s.o=e+1; return v.decode('latin1')

def parse_value(p, typ, depth, out):
    pad='  '*depth
    if typ==1: out.append("INT %d"%p.u32())
    elif typ==2: out.append("FLOAT %g"%p.f32())
    elif typ==3: out.append('STR "%s"'%p.cstr())
    elif typ==4: out.append('LSTR "%s"'%p.cstr())
    elif typ==5: out.append("PAIR (%g,%g)"%(p.f32(),p.f32()))
    elif typ==6: out.append("VEC (%g,%g,%g)"%(p.f32(),p.f32(),p.f32()))
    elif typ==13: out.append("NAME #%08x"%p.u32())
    elif typ==10:  # STRUCT: components until NONE(0)
        out.append("STRUCT {")
        parse_struct(p, depth+1, out)
        out.append(pad+"}")
    elif typ==12:  # ARRAY
        out.append("ARRAY[...]"); 
        raise StopIteration("array-unhandled@%x"%p.o)
    else:
        raise ValueError("unknown type 0x%x @0x%x"%(typ,p.o-1))

def parse_struct(p, depth, out):
    pad='  '*depth
    while True:
        if p.o>=len(p.d): out.append(pad+"<EOF>"); return
        typ=p.u8()
        if typ==0: return  # end of struct
        namehash=p.u32()
        line=pad+"#%08x : %s "%(namehash, T.get(typ,'?%d'%typ))
        sub=[]
        parse_value(p, typ, depth, sub)
        out.append(line+sub[0])
        out.extend(sub[1:])

# try a range of start offsets, pick the first that parses furthest
best=None
for start in range(0x46, 0x60):
    try:
        p=P(d,start); out=[]; parse_struct(p,0,out)
        if p.o>=len(d)-4:
            best=(start,out,p.o); break
        if best is None or p.o>best[2]: best=(start,out,p.o)
    except Exception as e:
        if best is None: best=(start,["ERR %s"%e],p.o)
        continue
start,out,end=best
print("BEST start=0x%x parsed_to=0x%x / 0x%x"%(start,end,len(d)))
print("\n".join(out[:120]))
