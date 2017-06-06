#!/usr/bin/env bash

function die() {
  echo $*
  exit 1
}

# Initialize coverage.out
echo "mode: count" > coverage.out

# Initialize error tracking
ERROR=""

# Test each package and append coverage profile info to coverage.out
for pkg in `find . -depth -name \*.go |
    grep -ve '^./vendor/' |
    grep -v integration_tests |
    sed -e 's/^\.\/\(\(.*\)\/\)\{0,1\}[^/]*$/github.com\/atlassian\/smith\/\2/' |
    sort -u`
do
    echo "Testing $pkg"
    go test -v -covermode=count -coverprofile=coverage_tmp.out "$pkg" || ERROR="Error testing $pkg"
    tail -n +2 coverage_tmp.out >> coverage.out 2> /dev/null ||:
done

rm -f coverage_tmp.out

if [ ! -z "$ERROR" ]
then
    die "Encountered error, last error was: $ERROR"
fi
