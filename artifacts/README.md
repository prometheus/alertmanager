# Generating amtool artifacts

Amtool comes with the option to create a number of ease-of-use artifacts.

    go run generate_amtool_artifacts.go

## Bash completion

The bash completion file can be added to `/etc/bash_completion.d/`.

## Man pages

Man pages can be added to the man directory of your choice

    cp artifacts/*.1 /usr/local/share/man/man1/
    sudo mandb

Then you should be able to view the man pages as expected.
