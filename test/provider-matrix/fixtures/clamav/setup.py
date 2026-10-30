"""Prepare a disposable real ClamAV scanner without downloading signatures."""
import fcntl
import hashlib
import os
from pathlib import Path
import subprocess

# Reproduce the same POSIX log lock held by the managed FreshClam updater.
with open('/var/log/clamav/freshclam.log', 'a') as log:
    fcntl.lockf(log, fcntl.LOCK_EX | fcntl.LOCK_NB)
    result = subprocess.run(['freshclam', '--quiet'], capture_output=True, text=True)
    assert result.returncode == 2
    assert 'Failed to lock the log file' in result.stdout + result.stderr

payload = b'remotr harmless scanner fixture\n'
# HDB signatures require MD5; this is a harmless scanner fixture, not security hashing.
Path('/var/lib/clamav/remotr.hdb').write_text(
    f'{hashlib.md5(payload).hexdigest()}:{len(payload)}:Remotr.TestFixture\n')
Path('/home/remotr-cron-private').mkdir()
os.chmod('/home/remotr-cron-private', 0o700)
Path('/etc/clamav/clamd.conf').write_text(
    'LocalSocket /tmp/remotr-clamd.sock\nUser clamav\n'
    'DatabaseDirectory /var/lib/clamav\nLogFile /tmp/remotr-clamd.log\n')
