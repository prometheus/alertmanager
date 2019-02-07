#!/bin/sh

set -ex

rm -f elm.js
rm -Rf elm-stuff
rm -Rf elm_home/0.19.0/package/elm-explorations/test

mkdir -p elm_home
mkdir -p elm_home/0.19.0/package

mkdir -p elm_home/0.19.0/package/elm-explorations/test/1.0.1
rsync -va ../ elm_home/0.19.0/package/elm-explorations/test/1.0.1/ --exclude tests --exclude elm-stuff --exclude .git

if which runhaskell; then
    # this produces ./versions.dat, but that file
    # is checked in, so it's fine to skip regenerating if
    # haskell is not available (such as on CI)
    runhaskell MakeTestRegistry.hs
fi
cp ./versions.dat elm_home/0.19.0/package/versions.dat

export ELM_HOME="$(pwd)"/elm_home

elm make src/Main.elm --output elm.js

node elm.js
