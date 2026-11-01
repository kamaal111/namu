import pathlib, shutil, sys
p=pathlib.Path(sys.argv[1])
assert not p.is_symlink()
if p.exists(): shutil.rmtree(p)
