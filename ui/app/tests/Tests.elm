module Tests exposing (..)

import Test exposing (..)
import Filter
import Match


all : Test
all =
    describe "Tests"
        [ utils
        , match
        ]


utils : Test
utils =
    describe "Utils"
        [ Filter.all
        ]


match : Test
match =
    describe "Match"
        [ Match.all
        ]
