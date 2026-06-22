import json
import subprocess


def get_logs():

    next = None

    while True:

        cmd = ["remotr", "logs", "list", "--since", "1h", "--json"]

        if next is not None:
            cmd.extend(["--cursor", next])

        try:
            command = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
            )

            data = json.loads(command.stdout)

            print(data)

            next = data["next_cursor"]

        except KeyError:
            break


if __name__ == "__main__":
    get_logs()
