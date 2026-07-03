#!/usr/bin/env python3
# THUG2 .ske.xbx skeleton parse + forward kinematics (world bind matrices). numpy.
import struct
import numpy as np
def quat_to_mat(q):  # q=(x,y,z,w)
    x,y,z,w=q; m=np.eye(4)
    m[0,0]=1-2*(y*y+z*z); m[0,1]=2*(x*y-z*w);   m[0,2]=2*(x*z+y*w)
    m[1,0]=2*(x*y+z*w);   m[1,1]=1-2*(x*x+z*z); m[1,2]=2*(y*z-x*w)
    m[2,0]=2*(x*z-y*w);   m[2,1]=2*(y*z+x*w);   m[2,2]=1-2*(x*x+y*y)
    return m
def T(v): m=np.eye(4); m[0,3],m[1,3],m[2,3]=v[0],v[1],v[2]; return m
def parse_ske(path):
    d=open(path,'rb').read()
    ver,flags,nb=struct.unpack_from('<iIi',d,0); o=12
    names=list(struct.unpack_from('<%dI'%nb,d,o)); o+=4*nb
    parents=list(struct.unpack_from('<%dI'%nb,d,o)); o+=4*nb
    o+=4*nb  # flips
    quats=[]; poss=[]
    for i in range(nb):
        q=struct.unpack_from('<4f',d,o); o+=16
        v=struct.unpack_from('<4f',d,o); o+=16
        quats.append(q); poss.append(v[:3])
    return nb,names,parents,quats,poss
def local_mats(quats,poss):
    return [T(poss[i])@quat_to_mat(quats[i]) for i in range(len(poss))]
def world_mats(parents,locals_):
    W=[None]*len(parents); idx={}
    # parents given by checksum; build name->index from order
    return W
if __name__=='__main__':
    import sys
    nb,names,parents,quats,poss=parse_ske('game-pristine-us/Data/skeletons/THPS6_Human.ske.xbx')
    name2idx={names[i]:i for i in range(nb)}
    L=local_mats(quats,poss)
    W=[None]*nb
    for i in range(nb):
        p=parents[i]
        if p==0 or p not in name2idx: W[i]=L[i]
        else: W[i]=W[name2idx[p]]@L[i]
    print("bone world positions (raw THUG coords):")
    for i in range(nb):
        wp=W[i][:3,3]
        print("  %2d #%08x parent=%2s  world=(%.1f, %.1f, %.1f)"%(
            i,names[i], name2idx.get(parents[i],'-'), wp[0],wp[1],wp[2]))
