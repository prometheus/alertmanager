module StringUtils exposing (testLinkify)

import Expect
import Test exposing (..)
import Utils.String exposing (linkify)


testLinkify : Test
testLinkify =
    describe "linkify"
        [ test "should linkify a url in the middle" <|
            \() ->
                Expect.equal (linkify "word1 http://url word2")
                    [ Err "word1 ", Ok "http://url", Err " word2" ]
        , test "should linkify a url in the beginning" <|
            \() ->
                Expect.equal (linkify "http://url word1 word2")
                    [ Ok "http://url", Err " word1 word2" ]
        , test "should linkify a url in the end" <|
            \() ->
                Expect.equal (linkify "word1 word2 http://url")
                    [ Err "word1 word2 ", Ok "http://url" ]
        ]
