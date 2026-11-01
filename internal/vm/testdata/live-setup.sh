printf guest >> tracked.txt
git add tracked.txt
git -c user.name=Smoke -c user.email=smoke@example.com commit -qm guest
uname -s > guest-platform
