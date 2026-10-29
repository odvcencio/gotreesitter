"""Fail-closed helpers for the W5 private C-oracle cache."""
import hashlib,json,os,pathlib,stat

OVERRIDES=('GTS_PARITY_REPO_ROOT','GTS_PARITY_REPO_CACHE','GTS_PARITY_EXTRA_LOCK')

def require(ok,message):
 if not ok: raise ValueError(message)

def digest(path):
 p=pathlib.Path(path);info=p.lstat();require(stat.S_ISREG(info.st_mode) and not stat.S_ISLNK(info.st_mode),'not a regular file: '+str(p))
 return hashlib.sha256(p.read_bytes()).hexdigest()

def check_env(env):
 require(not any(env.get(x) for x in OVERRIDES),'untrusted grammar source override')

def source_fingerprints(inputs):
 return {str(pathlib.Path(p)):digest(p) for p in sorted(inputs)}

def identity_fields(identity):
 required=('contract','transport','binding_module','binding_version','binding_commit','runtime_version','runtime_commit','runtime_linkage','sources_manifest_sha256','language','grammar_repo','grammar_commit','grammar_linkage','grammar_compile_flags','compiler_path','compiler_version','grammar_artifact_sha256')
 require(all(isinstance(identity.get(k),str) and identity[k] for k in required),'incomplete oracle identity')
 return {k:identity[k] for k in required}
