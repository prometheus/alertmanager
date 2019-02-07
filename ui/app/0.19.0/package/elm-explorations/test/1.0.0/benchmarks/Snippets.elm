module Snippets exposing (..)

import Expect exposing (Expectation)
import Fuzz exposing (Fuzzer)
import Test exposing (Test, fuzz)


intPass : Test
intPass =
    fuzz Fuzz.int "(passes) int" <|
        \_ ->
            Expect.pass


intFail : Test
intFail =
    fuzz Fuzz.int "(fails) int" <|
        \numbers ->
            Expect.fail "Failed"


intRangePass : Test
intRangePass =
    fuzz (Fuzz.intRange 10 100) "(passes) intRange" <|
        \_ ->
            Expect.pass


intRangeFail : Test
intRangeFail =
    fuzz (Fuzz.intRange 10 100) "(fails) intRange" <|
        \numbers ->
            Expect.fail "Failed"


stringPass : Test
stringPass =
    fuzz Fuzz.string "(passes) string" <|
        \_ ->
            Expect.pass


stringFail : Test
stringFail =
    fuzz Fuzz.string "(fails) string" <|
        \numbers ->
            Expect.fail "Failed"


floatPass : Test
floatPass =
    fuzz Fuzz.float "(passes) float" <|
        \_ ->
            Expect.pass


floatFail : Test
floatFail =
    fuzz Fuzz.float "(fails) float" <|
        \numbers ->
            Expect.fail "Failed"


boolPass : Test
boolPass =
    fuzz Fuzz.bool "(passes) bool" <|
        \_ ->
            Expect.pass


boolFail : Test
boolFail =
    fuzz Fuzz.bool "(fails) bool" <|
        \numbers ->
            Expect.fail "Failed"


charPass : Test
charPass =
    fuzz Fuzz.char "(passes) char" <|
        \_ ->
            Expect.pass


charFail : Test
charFail =
    fuzz Fuzz.char "(fails) char" <|
        \numbers ->
            Expect.fail "Failed"


listIntPass : Test
listIntPass =
    fuzz (Fuzz.list Fuzz.int) "(passes) list of int" <|
        \_ ->
            Expect.pass


listIntFail : Test
listIntFail =
    fuzz (Fuzz.list Fuzz.int) "(fails) list of int" <|
        {- The empty list is the first value the list shrinker will try.
           If we immediately fail on that example than we're not doing a lot of shrinking.
        -}
        Expect.notEqual []


maybeIntPass : Test
maybeIntPass =
    fuzz (Fuzz.maybe Fuzz.int) "(passes) maybe of int" <|
        \_ ->
            Expect.pass


maybeIntFail : Test
maybeIntFail =
    fuzz (Fuzz.maybe Fuzz.int) "(fails) maybe of int" <|
        \numbers ->
            Expect.fail "Failed"


resultPass : Test
resultPass =
    fuzz (Fuzz.result Fuzz.string Fuzz.int) "(passes) result of string and int" <|
        \_ ->
            Expect.pass


resultFail : Test
resultFail =
    fuzz (Fuzz.result Fuzz.string Fuzz.int) "(fails) result of string and int" <|
        \numbers ->
            Expect.fail "Failed"


mapPass : Test
mapPass =
    fuzz even "(passes) map" <|
        \_ -> Expect.pass


mapFail : Test
mapFail =
    fuzz even "(fails) map" <|
        \_ -> Expect.fail "Failed"


andMapPass : Test
andMapPass =
    fuzz person "(passes) andMap" <|
        \_ -> Expect.pass


andMapFail : Test
andMapFail =
    fuzz person "(fails) andMap" <|
        \_ -> Expect.fail "Failed"


map5Pass : Test
map5Pass =
    fuzz person2 "(passes) map5" <|
        \_ -> Expect.pass


map5Fail : Test
map5Fail =
    fuzz person2 "(fails) map5" <|
        \_ -> Expect.fail "Failed"


andThenPass : Test
andThenPass =
    fuzz (variableList 2 5 Fuzz.int) "(passes) andThen" <|
        \_ -> Expect.pass


andThenFail : Test
andThenFail =
    fuzz (variableList 2 5 Fuzz.int) "(fails) andThen" <|
        \_ -> Expect.fail "Failed"


conditionalPass : Test
conditionalPass =
    fuzz evenWithConditional "(passes) conditional" <|
        \_ -> Expect.pass


conditionalFail : Test
conditionalFail =
    fuzz evenWithConditional "(fails) conditional" <|
        \_ -> Expect.fail "Failed"


type alias Person =
    { firstName : String
    , lastName : String
    , age : Int
    , nationality : String
    , height : Float
    }


person : Fuzzer Person
person =
    Fuzz.map Person Fuzz.string
        |> Fuzz.andMap Fuzz.string
        |> Fuzz.andMap Fuzz.int
        |> Fuzz.andMap Fuzz.string
        |> Fuzz.andMap Fuzz.float


person2 : Fuzzer Person
person2 =
    Fuzz.map5 Person
        Fuzz.string
        Fuzz.string
        Fuzz.int
        Fuzz.string
        Fuzz.float


even : Fuzzer Int
even =
    Fuzz.map ((*) 2) Fuzz.int


variableList : Int -> Int -> Fuzzer a -> Fuzzer (List a)
variableList min max item =
    Fuzz.intRange min max
        |> Fuzz.andThen (\length -> List.repeat length item |> sequence)


sequence : List (Fuzzer a) -> Fuzzer (List a)
sequence fuzzers =
    List.foldl
        (Fuzz.map2 (::))
        (Fuzz.constant [])
        fuzzers


evenWithConditional : Fuzzer Int
evenWithConditional =
    Fuzz.intRange 1 6
        |> Fuzz.conditional
            { retries = 3
            , fallback = (+) 1
            , condition = \n -> (n % 2) == 0
            }
