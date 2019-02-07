module Test.Array exposing (tests)

import Array
import Basics exposing (..)
import List exposing ((::))
import Maybe exposing (..)
import Test exposing (..)
import Fuzz exposing (Fuzzer, intRange)
import Expect


tests : Test
tests =
    describe "Array"
        [ initTests
        , isEmptyTests
        , lengthTests
        , getSetTests
        , conversionTests
        , transformTests
        , sliceTests
        , runtimeCrashTests
        ]


{-| > 33000 elements requires 3 levels in the tree
-}
defaultSizeRange : Fuzzer Int
defaultSizeRange =
    (intRange 1 33000)


initTests : Test
initTests =
    describe "Initialization"
        [ fuzz defaultSizeRange "initialize" <|
            \size ->
                toList (initialize size identity)
                    |> Expect.equal (List.range 0 (size - 1))
        , fuzz defaultSizeRange "push" <|
            \size ->
                List.foldl push empty (List.range 0 (size - 1))
                    |> Expect.equal (initialize size identity)
        , test "initialize non-identity" <|
            \() ->
                toList (initialize 4 (\n -> n * n))
                    |> Expect.equal [ 0, 1, 4, 9 ]
        , test "initialize empty" <|
            \() ->
                toList (initialize 0 identity)
                    |> Expect.equal []
        , test "initialize negative" <|
            \() ->
                toList (initialize -2 identity)
                    |> Expect.equal []
        ]


isEmptyTests : Test
isEmptyTests =
    describe "isEmpty"
        [ test "all empty arrays are equal" <|
            \() ->
                Expect.equal empty (fromList [])
        , test "empty array" <|
            \() ->
                isEmpty empty
                    |> Expect.equal True
        , test "empty converted array" <|
            \() ->
                isEmpty (fromList [])
                    |> Expect.equal True
        , test "non-empty array" <|
            \() ->
                isEmpty (fromList [ 1 ])
                    |> Expect.equal False
        ]


lengthTests : Test
lengthTests =
    describe "Length"
        [ test "empty array" <|
            \() ->
                length empty
                    |> Expect.equal 0
        , fuzz defaultSizeRange "non-empty array" <|
            \size ->
                length (initialize size identity)
                    |> Expect.equal size
        , fuzz defaultSizeRange "push" <|
            \size ->
                length (push size (initialize size identity))
                    |> Expect.equal (size + 1)
        , fuzz defaultSizeRange "append" <|
            \size ->
                length (append (initialize size identity) (initialize (size // 2) identity))
                    |> Expect.equal (size + (size // 2))
        , fuzz defaultSizeRange "set does not increase" <|
            \size ->
                length (set (size // 2) 1 (initialize size identity))
                    |> Expect.equal size
        , fuzz (intRange 100 10000) "big slice" <|
            \size ->
                length (slice 35 -35 (initialize size identity))
                    |> Expect.equal (size - 70)
        , fuzz2 (intRange -32 -1) (intRange 100 10000) "small slice end" <|
            \n size ->
                length (slice 0 n (initialize size identity))
                    |> Expect.equal (size + n)
        ]


getSetTests : Test
getSetTests =
    describe "Get and set"
        [ fuzz2 defaultSizeRange defaultSizeRange "can retrieve element" <|
            \x y ->
                let
                    n =
                        min x y

                    size =
                        max x y
                in
                    get n (initialize (size + 1) identity)
                        |> Expect.equal (Just n)
        , fuzz2 (intRange 1 50) (intRange 100 33000) "out of bounds retrieval returns nothing" <|
            \n size ->
                let
                    arr =
                        initialize size identity
                in
                    ( get (negate n) arr
                    , get (size + n) arr
                    )
                        |> Expect.equal ( Nothing, Nothing )
        , fuzz2 defaultSizeRange defaultSizeRange "set replaces value" <|
            \x y ->
                let
                    n =
                        min x y

                    size =
                        max x y
                in
                    get n (set n 5 (initialize (size + 1) identity))
                        |> Expect.equal (Just 5)
        , fuzz2 (intRange 1 50) defaultSizeRange "set out of bounds returns original array" <|
            \n size ->
                let
                    arr =
                        initialize size identity
                in
                    set (negate n) 5 arr
                        |> set (size + n) 5
                        |> Expect.equal arr
        , test "Retrieval works from tail" <|
            \() ->
                get 1030 (set 1030 5 (initialize 1035 identity))
                    |> Expect.equal (Just 5)
        ]


conversionTests : Test
conversionTests =
    describe "Conversion"
        [ fuzz defaultSizeRange "back and forth" <|
            \size ->
                let
                    ls =
                        List.range 0 (size - 1)
                in
                    toList (fromList ls)
                        |> Expect.equal ls
        , fuzz defaultSizeRange "indexed" <|
            \size ->
                toIndexedList (initialize size ((+) 1))
                    |> Expect.equal (toList (initialize size (\idx -> ( idx, idx + 1 ))))
        ]


transformTests : Test
transformTests =
    describe "Transform"
        [ fuzz defaultSizeRange "foldl" <|
            \size ->
                foldl (::) [] (initialize size identity)
                    |> Expect.equal (List.reverse (List.range 0 (size - 1)))
        , fuzz defaultSizeRange "foldr" <|
            \size ->
                foldr (\n acc -> n :: acc) [] (initialize size identity)
                    |> Expect.equal (List.range 0 (size - 1))
        , fuzz defaultSizeRange "filter" <|
            \size ->
                toList (filter (\a -> a % 2 == 0) (initialize size identity))
                    |> Expect.equal (List.filter (\a -> a % 2 == 0) (List.range 0 (size - 1)))
        , fuzz defaultSizeRange "map" <|
            \size ->
                map ((+) 1) (initialize size identity)
                    |> Expect.equal (initialize size ((+) 1))
        , fuzz defaultSizeRange "indexedMap" <|
            \size ->
                indexedMap (*) (repeat size 5)
                    |> Expect.equal (initialize size ((*) 5))
        , fuzz defaultSizeRange "push appends one element" <|
            \size ->
                push size (initialize size identity)
                    |> Expect.equal (initialize (size + 1) identity)
        , fuzz (intRange 1 1050) "append" <|
            \size ->
                append (initialize size identity) (initialize size ((+) size))
                    |> Expect.equal (initialize (size * 2) identity)
        , fuzz2 defaultSizeRange (intRange 1 32) "small appends" <|
            \s1 s2 ->
                append (initialize s1 identity) (initialize s2 ((+) s1))
                    |> Expect.equal (initialize (s1 + s2) identity)
        , fuzz (defaultSizeRange) "toString" <|
            \size ->
                let
                    ls =
                        List.range 0 size
                in
                    Array.toString (fromList ls)
                        |> Expect.equal ("Array " ++ Basics.toString ls)
        ]


sliceTests : Test
sliceTests =
    let
        smallSample =
            fromList (List.range 1 8)
    in
        describe "Slice"
            [ fuzz2 (intRange -50 -1) (intRange 100 33000) "both" <|
                \n size ->
                    slice (abs n) n (initialize size identity)
                        |> Expect.equal (initialize (size + n + n) (\idx -> idx - n))
            , fuzz2 (intRange -50 -1) (intRange 100 33000) "left" <|
                \n size ->
                    let
                        arr =
                            initialize size identity
                    in
                        slice (abs n) (length arr) arr
                            |> Expect.equal (initialize (size + n) (\idx -> idx - n))
            , fuzz2 (intRange -50 -1) (intRange 100 33000) "right" <|
                \n size ->
                    slice 0 n (initialize size identity)
                        |> Expect.equal (initialize (size + n) identity)
            , fuzz defaultSizeRange "slicing all but the last item" <|
                \size ->
                    initialize size identity
                        |> slice -1 size
                        |> toList
                        |> Expect.equal [ size - 1 ]
            , test "both small" <|
                \() ->
                    toList (slice 2 5 smallSample)
                        |> Expect.equal (List.range 3 5)
            , test "start small" <|
                \() ->
                    toList (slice 2 (length smallSample) smallSample)
                        |> Expect.equal (List.range 3 8)
            , test "negative" <|
                \() ->
                    toList (slice -5 -2 smallSample)
                        |> Expect.equal (List.range 4 6)
            , test "impossible" <|
                \() ->
                    toList (slice -1 -2 smallSample)
                        |> Expect.equal []
            , test "crash" <|
                \() ->
                    Array.repeat (33 * 32) 1
                        |> Array.slice 0 1
                        |> Expect.equal (Array.repeat 1 1)
            ]


runtimeCrashTests : Test
runtimeCrashTests =
    describe "Runtime crashes in core"
        [ test "magic slice" <|
            \() ->
                let
                    n =
                        10
                in
                    initialize (4 * n) identity
                        |> slice n (4 * n)
                        |> slice n (3 * n)
                        |> slice n (2 * n)
                        |> slice n n
                        |> \a -> Expect.equal a a
        , test "magic slice 2" <|
            \() ->
                let
                    ary =
                        fromList <| List.range 0 32

                    res =
                        append (slice 1 32 ary) (slice (32 + 1) -1 ary)
                in
                    Expect.equal res res
        , test "magic append" <|
            \() ->
                let
                    res =
                        append (initialize 1 (always 1))
                            (initialize (32 ^ 2 - 1 * 32 + 1) (\i -> i))
                in
                    Expect.equal res res
        ]
