import hashlib
import pathlib
import sys

prefix = b'{ a[(1),(2),""]=value }\n#'
source = prefix + b' ' * (1048576 - len(prefix) - 1) + b'\n'
assert hashlib.sha256(source).hexdigest() == 'ce0c557309116ae0c53bbf01e2f5f98846d2ac95e659f143872b8ccdc89a36c3'
pathlib.Path(sys.argv[1]).write_bytes(source)
