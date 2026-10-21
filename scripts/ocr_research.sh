#!/bin/bash
f="$1"; b=$(basename "$f" .pdf); mkdir -p "img/$b"
pdftoppm -r 200 -png "$f" "img/$b/p"
for p in img/$b/p-*.png; do n=$(basename $p .png); tesseract "$p" "img/$b/$n" -l rus+eng --psm 3 --oem 1 >/dev/null 2>&1; done
for t in $(ls img/$b/p-*.txt | sort -V); do echo "=== PAGE ${t##*p-}" ; cat "$t"; done > "/home/claude/work/research/$b.txt"
echo done $b
