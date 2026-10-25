#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <document.pdf>" >&2
  exit 2
fi

input=$1
if [[ ! -f "$input" ]]; then
  echo "File not found: $input" >&2
  exit 1
fi

for command in pdftoppm tesseract; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Required command not found: $command" >&2
    exit 1
  fi
done

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
project_root=$(cd -- "$script_dir/.." && pwd -P)
input=$(cd -- "$(dirname -- "$input")" && pwd -P)/$(basename -- "$input")
file_name=$(basename -- "$input")
base_name=${file_name%.*}
image_dir="$project_root/img/$base_name"
output_dir="$project_root/data/research"
output_file="$output_dir/$base_name.txt"

mkdir -p "$image_dir" "$output_dir"
# Remove output from an earlier run so pages deleted from a PDF cannot leak into the new result.
find "$image_dir" -maxdepth 1 -type f \( -name 'p-*.png' -o -name 'p-*.txt' \) -delete

pdftoppm -r 200 -png "$input" "$image_dir/p"

mapfile -d '' images < <(find "$image_dir" -maxdepth 1 -type f -name 'p-*.png' -print0 | sort -zV)
if [[ ${#images[@]} -eq 0 ]]; then
  echo "No pages were rendered from: $input" >&2
  exit 1
fi

for page_image in "${images[@]}"; do
  page_name=$(basename -- "$page_image" .png)
  tesseract "$page_image" "$image_dir/$page_name" -l rus+eng --psm 3 --oem 1 >/dev/null 2>&1
done

mapfile -d '' texts < <(find "$image_dir" -maxdepth 1 -type f -name 'p-*.txt' -print0 | sort -zV)
: > "$output_file"
for page_text in "${texts[@]}"; do
  page_name=$(basename -- "$page_text" .txt)
  printf '=== PAGE %s\n' "${page_name#p-}" >> "$output_file"
  cat "$page_text" >> "$output_file"
  printf '\n' >> "$output_file"
done

echo "OCR text written to: $output_file"
