#!/bin/sh

cd "$(dirname "$0")"
set -e


elm-package install -y

VERSION_DIR="$(ls elm-stuff/packages/elm-lang/core/)"
CORE_PACKAGE_DIR="elm-stuff/packages/elm-lang/core/$VERSION_DIR"
CORE_GIT_DIR="$(dirname $PWD)"

echo "Linking $CORE_PACKAGE_DIR to $CORE_GIT_DIR"
rm -rf $CORE_PACKAGE_DIR
ln -s $CORE_GIT_DIR $CORE_PACKAGE_DIR

elm-make --yes --output test.js Main.elm

elm-test Main.elm
