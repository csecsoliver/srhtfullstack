import re
from typing import List, Tuple

_git_generated_prefixes = (
    "Signed-off-by: ",
    "(cherry picked from commit ",
)

def commit_trailers(message: str) -> List[Tuple[str, str]]:
    """
    Extract the trailers from a commit message. Return a list of pairs of
    (name, value).

    This borrows a large amount of logic from git core (trailer.c).
    """
    lines = message.strip().splitlines()

    # The first paragraph is the title and cannot be trailers
    while lines and lines[0] != '':
        del lines[0]

    recognized_prefix = False
    only_spaces = True
    trailer_lines = non_trailer_lines = 0
    possible_continuation_lines = 0

    # Get the start of the trailers by looking starting from the end for a
    # blank line before a set of non-blank lines that (i) are all trailers, or
    # (ii) contains at least one Git-generated trailer and consists of at least
    # 25% trailers.
    i = len(lines) - 1
    while i >= 0:
        line = lines[i]

        if not line.strip():
            # blank line
            if only_spaces:
                i -= 1
                continue
            if recognized_prefix and trailer_lines * 3 >= non_trailer_lines:
                i += 1
                break
            if trailer_lines > 0 and non_trailer_lines == 0:
                i += 1
                break
            return []

        only_spaces = False

        if any(line.startswith(p) for p in _git_generated_prefixes):
            trailer_lines += 1
            possible_continuation_lines = 0
            recognized_prefix = True
        elif re.search(r"^[A-Za-z\d][A-Za-z\d-]*\s*:", line):
            trailer_lines += 1
            possible_continuation_lines = 0
        elif line[0] in (" ", "\t"):
            possible_continuation_lines += 1
        else:
            non_trailer_lines += 1 + possible_continuation_lines
            possible_continuation_lines = 0
        i -= 1

    # Iterate over all remaining lines and collect trailer names and values.
    # If a line does not match a trailer and starts with a space or tab, its
    # contents are appended to the current trailer value.
    trailers = []
    name = value = None

    for line in lines[i:]:
        match = re.match(r"^([A-Za-z\d][A-Za-z\d-]*)\s*:\s*(.*)$", line)
        if match:
            if name is not None and value is not None:
                trailers.append((name, value))
            name = match[1]
            value = match[2]
        elif name is not None and value is not None and line[0] in (" ", "\t"):
            # continuation line
            value += "\n" + line
    if name is not None and value is not None:
        trailers.append((name, value))

    return trailers
