module Tests exposing (..)

import Test exposing (..)
import Filter


all : Test
all =
    describe "Tests"
        [ utils
        ]


utils : Test
utils =
    describe "Utils"
        [ Filter.all
        ]
