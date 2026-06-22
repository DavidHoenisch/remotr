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

            # The URL stores the next page in a json field called
            # "next_cursor"
            next = data["next_cursor"]

        # when there are no more events, the server will omit the
        # next_cursor field. Therefor, when the KeyError is raised
        # we can break out of the loop, we have gotten all the even
        # for the specified time range
        except KeyError:
            break


if __name__ == "__main__":
    get_logs()
