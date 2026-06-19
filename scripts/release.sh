#!/bin/sh

set -e

export GITHUB_TOKEN=""
export RBW_ITEM="github goreleaser pat"

check_bin() {
  bin=$1

  if [[ ! -x $(which ${bin} 2>/dev/null) ]]; then
    echo "Missing ${bin} in your path.  Add to your path and/or install via your favorite package manager (pkg_add, brew, etc)"
    exit 1
  fi
}

check_bin goreleaser

if [[ -x $(which sysctl) ]] ; then
  ostype=$($(which sysctl) -a | grep kern.ostype | cut -f 2 -d =)
  if [ ${ostype} == "OpenBSD" ] ; then
    echo "We're on OpenBSD"
    check_bin rbw

    echo "Getting GITHUB_TOKEN from rbw"
    GITHUB_TOKEN=$(rbw get "${RBW_ITEM}")
  fi

else
  echo "Assuming we're on MacOS"
  GITHUB_TOKEN=$(security find-generic-password -w -a ${USER} -D "application password" -s "github_goreleaser_token")
fi

goreleaser release
