#!/bin/bash

# Replace harsh borders with transparent or subtle ones
find src -name "*.tsx" -type f -exec sed -i \
  -e 's/border-line-faint/border-transparent/g' \
  -e 's/border-line-strong/border-transparent/g' \
  -e 's/border border-line/border-transparent/g' \
  -e 's/border-line/border-transparent/g' \
  -e 's/border-b border-transparent/border-b border-transparent/g' \
  -e 's/border-t border-transparent/border-t border-transparent/g' \
  -e 's/border-l-4/border-l-4/g' \
  -e 's/border-l-2/border-l-2/g' \
  -e 's/border-r border-transparent/border-r border-transparent/g' \
  {} +

# Keep colored borders (success/error/warning/info)
find src -name "*.tsx" -type f -exec sed -i \
  -e 's/border-transparent\/30/border-success\/30/g' \
  -e 's/border-transparent-subtle/border-success-subtle/g' \
  {} +
