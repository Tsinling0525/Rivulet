#!/usr/bin/env python3
"""Convert an image file to a minimal LaTeX document that embeds the image."""
import os
import sys


def main() -> None:
    if len(sys.argv) < 2:
        sys.stderr.write("Usage: img_to_latex.py <image_path>\n")
        sys.exit(1)
    img_path = sys.argv[1]
    name = os.path.basename(img_path)
    tex = (
        "\\documentclass{article}\n"
        "\\usepackage{graphicx}\n"
        "\\begin{document}\n"
        f"\\includegraphics[width=\\linewidth]{{{name}}}\n"
        "\\end{document}\n"
    )
    print(tex)


if __name__ == "__main__":
    main()
