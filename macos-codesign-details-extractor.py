import argparse
import subprocess
import sys


class MyParser(argparse.ArgumentParser):
    def error(self, message):
        sys.stderr.write("error: %s\n" % message)
        self.print_help()
        sys.exit(3)


parser = MyParser(
    description="Return the first valid Apple certificate organisational unit found in your macOS keychain"
)
parser.add_argument(
    "--common-name",
    default="Developer ID Application",
    help='certificate common-name prefix to search for (default: "Developer ID Application")',
)
args = parser.parse_args()

subject = subprocess.check_output(
    f'security find-certificate -c "{args.common_name}" -p | /usr/bin/openssl x509 -subject',
    universal_newlines=True,
    shell=True,
    stderr=subprocess.STDOUT,
)

begin = "/OU="
begin_index = subject.find(begin)
if begin_index == -1:
    raise Exception(
        "Failed to find organisational unit when looking for the following string in the certificate subject:",
        begin,
    )

end = "/O"
end_index = subject.find(end, begin_index + len(begin))
if end_index == -1:
    raise Exception(
        "Only organisational unit start marker '/OU=' was found. Failed to find end marker '/O' in certificate subject:",
        subject,
    )

organisational_unit = subject[begin_index + len(begin) : end_index]
sys.stdout.write(organisational_unit)
