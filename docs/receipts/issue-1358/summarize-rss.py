import re,json,pathlib,statistics,sys
root=pathlib.Path(sys.argv[1]);out={}
for mode in ['full','insert','noedit']:
 samples=[]
 for seed in range(1,21):
  sample={'seed':seed}
  for role in ['base','head','c']:
   path=root/f'{mode}-{role}-{seed}.time'
   raw=path.read_text()
   assert re.search(r'Exit status: 0\s*$',raw),path
   sample[role+'_rss_KiB']=int(re.search(r'Maximum resident set size \(kbytes\): (\d+)',raw)[1])
  sample['base_Go_C_RSS']=sample['base_rss_KiB']/sample['c_rss_KiB']
  sample['head_Go_C_RSS']=sample['head_rss_KiB']/sample['c_rss_KiB']
  samples.append(sample)
 out[mode]={'samples':samples,'medians':{k:statistics.median(s[k] for s in samples) for k in samples[0] if k!='seed'},'maxima':{k:max(s[k] for s in samples) for k in samples[0] if k.endswith('rss_KiB')}}
 before=out[mode]['medians']['base_rss_KiB'];after=out[mode]['medians']['head_rss_KiB']
 print(mode,f'RSS KiB {before} -> {after}; delta={(after/before-1)*100:.2f}%; Go/C {out[mode]["medians"]["base_Go_C_RSS"]:.3f} -> {out[mode]["medians"]["head_Go_C_RSS"]:.3f}; max head {out[mode]["maxima"]["head_rss_KiB"]} KiB')
x={'fixture_bytes':1048576,'fixture_SHA256':'ce0c557309116ae0c53bbf01e2f5f98846d2ac95e659f143872b8ccdc89a36c3','repetitions_per_process':10,'seeds':20,'ordering':'base-C-head and head-C-base alternate by seed','measurement':'Maximum whole-process RSS, including runtime and live starting/result trees; native C has no Go binding','modes':out}
(root.parent/'rss-summary.json').write_text(json.dumps(x,indent=2)+'\n')
