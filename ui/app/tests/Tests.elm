module Tests exposing (..)

import Test exposing (..)
import Filter
import GroupBar


all : Test
all =
    describe "Tests"
        [ utils
        , groupBar
        ]


utils : Test
utils =
    describe "Utils"
        [ Filter.all
        ]


groupBar : Test
groupBar =
    describe "GroupBar"
        [ GroupBar.all
        ]
