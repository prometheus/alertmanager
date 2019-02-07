module Expect
    exposing
        ( Expectation
        , FloatingPointTolerance(..)
        , all
        , atLeast
        , atMost
        , equal
        , equalDicts
        , equalLists
        , equalSets
        , err
        , fail
        , false
        , greaterThan
        , lessThan
        , notEqual
        , notWithin
        , ok
        , onFail
        , pass
        , true
        , within
        )

{-| A library to create `Expectation`s, which describe a claim to be tested.


## Quick Reference

  - [`equal`](#equal) `(arg2 == arg1)`
  - [`notEqual`](#notEqual) `(arg2 /= arg1)`
  - [`lessThan`](#lessThan) `(arg2 < arg1)`
  - [`atMost`](#atMost) `(arg2 <= arg1)`
  - [`greaterThan`](#greaterThan) `(arg2 > arg1)`
  - [`atLeast`](#atLeast) `(arg2 >= arg1)`
  - [`true`](#true) `(arg == True)`
  - [`false`](#false) `(arg == False)`
  - [Floating Point Comparisons](#floating-point-comparisons)


## Basic Expectations

@docs Expectation, equal, notEqual, all


## Numeric Comparisons

@docs lessThan, atMost, greaterThan, atLeast


## Floating Point Comparisons

These functions allow you to compare `Float` values up to a specified rounding error, which may be relative, absolute,
or both. For an in-depth look, see our [Guide to Floating Point Comparison](#guide-to-floating-point-comparison).

@docs FloatingPointTolerance, within, notWithin


## Booleans

@docs true, false


## Collections

@docs ok, err, equalLists, equalDicts, equalSets


## Customizing

These functions will let you build your own expectations.

@docs pass, fail, onFail


## Guide to Floating Point Comparison

In general, if you are multiplying, you want relative tolerance, and if you're adding,
you want absolute tolerance. If you are doing both, you want both kinds of tolerance,
or to split the calculation into smaller parts for testing.


### Absolute Tolerance

Let's say we want to figure out if our estimation of pi is precise enough.

Is `3.14` within `0.01` of `pi`? Yes, because `3.13 < pi < 3.15`.

    test "3.14 approximates pi with absolute precision" <| \_ ->
        3.14 |> Expect.within (Absolute 0.01) pi


### Relative Tolerance

What if we also want to know if our circle circumference estimation is close enough?

Let's say our circle has a radius of `r` meters. The formula for circle circumference is `C=2*r*pi`.
To make the calculations a bit easier ([ahem](https://tauday.com/tau-manifesto)), we'll look at half the circumference; `C/2=r*pi`.
Is `r * 3.14` within `0.01` of `r * pi`?
That depends, what does `r` equal? If `r` is `0.01`mm, or `0.00001` meters, we're comparing
`0.00001 * 3.14 - 0.01 < r * pi < 0.00001 * 3.14 + 0.01` or `-0.0099686 < 0.0000314159 < 0.0100314`.
That's a huge tolerance! A circumference that is _a thousand times longer_ than we expected would pass that test!

On the other hand, if `r` is very large, we're going to need many more digits of pi.
For an absolute tolerance of `0.01` and a pi estimation of `3.14`, this expectation only passes if `r < 2*pi`.

If we use a relative tolerance of `0.01` instead, the circle area comparison becomes much better. Is `r * 3.14` within
`1%` of `r * pi`? Yes! In fact, three digits of pi approximation is always good enough for a 0.1% relative tolerance,
as long as `r` isn't [too close to zero](https://en.wikipedia.org/wiki/Denormal_number).

    fuzz
        (floatRange 0.000001 100000)
        "Circle half-circumference with relative tolerance"
        (\r -> r * 3.14 |> Expect.within (Relative 0.001) (r * pi))


### Trouble with Numbers Near Zero

If you are adding things near zero, you probably want absolute tolerance. If you're comparing values between `-1` and `1`, you should consider using absolute tolerance.

For example: Is `1 + 2 - 3` within `1%` of `0`? Well, if `1`, `2` and `3` have any amount of rounding error, you might not get exactly zero. What is `1%` above and below `0`? Zero. We just lost all tolerance. Even if we hard-code the numbers, we might not get exactly zero; `0.1 + 0.2` rounds to a value just above `0.3`, since computers, counting in binary, cannot write down any of those three numbers using a finite number of digits, just like we cannot write `0.333...` exactly in base 10.

Another example is comparing values that are on either side of zero. `0.0001` is more than `100%` away from `-0.0001`. In fact, `infinity` is closer to `0.0001` than `0.0001` is to `-0.0001`, if you are using a relative tolerance. Twice as close, actually. So even though both `0.0001` and `-0.0001` could be considered very close to zero, they are very far apart relative to each other. The same argument applies for any number of zeroes.

-}

import Dict exposing (Dict)
import Set exposing (Set)
import Test.Expectation
import Test.Internal as Internal
import Test.Runner.Failure exposing (InvalidReason(..), Reason(..))


{-| The result of a single test run: either a [`pass`](#pass) or a
[`fail`](#fail).
-}
type alias Expectation =
    Test.Expectation.Expectation


{-| Passes if the arguments are equal.

    Expect.equal 0 (List.length [])

    -- Passes because (0 == 0) is True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because the expected value didn't split the space in "Betty Botter"
    String.split " " "Betty Botter bought some butter"
        |> Expect.equal [ "Betty Botter", "bought", "some", "butter" ]

    {-

    [ "Betty", "Botter", "bought", "some", "butter" ]
    ╷
    │ Expect.equal
    ╵
    [ "Betty Botter", "bought", "some", "butter" ]

    -}

Do not equate `Float` values; use [`within`](#within) instead.

-}
equal : a -> a -> Expectation
equal =
    equateWith "Expect.equal" (==)


{-| Passes if the arguments are not equal.

    -- Passes because (11 /= 100) is True
    90 + 10
        |> Expect.notEqual 11


    -- Fails because (100 /= 100) is False
    90 + 10
        |> Expect.notEqual 100

    {-

    100
    ╷
    │ Expect.notEqual
    ╵
    100

    -}

-}
notEqual : a -> a -> Expectation
notEqual =
    equateWith "Expect.notEqual" (/=)


{-| Passes if the second argument is less than the first.

    Expect.lessThan 1 (List.length [])

    -- Passes because (0 < 1) is True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because (0 < -1) is False
    List.length []
        |> Expect.lessThan -1


    {-

    0
    ╷
    │ Expect.lessThan
    ╵
    -1

    -}

Do not equate `Float` values; use [`notWithin`](#notWithin) instead.

-}
lessThan : comparable -> comparable -> Expectation
lessThan =
    compareWith "Expect.lessThan" (<)


{-| Passes if the second argument is less than or equal to the first.

    Expect.atMost 1 (List.length [])

    -- Passes because (0 <= 1) is True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because (0 <= -3) is False
    List.length []
        |> Expect.atMost -3

    {-

    0
    ╷
    │ Expect.atMost
    ╵
    -3

    -}

-}
atMost : comparable -> comparable -> Expectation
atMost =
    compareWith "Expect.atMost" (<=)


{-| Passes if the second argument is greater than the first.

    Expect.greaterThan -2 List.length []

    -- Passes because (0 > -2) is True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because (0 > 1) is False
    List.length []
        |> Expect.greaterThan 1

    {-

    0
    ╷
    │ Expect.greaterThan
    ╵
    1

    -}

-}
greaterThan : comparable -> comparable -> Expectation
greaterThan =
    compareWith "Expect.greaterThan" (>)


{-| Passes if the second argument is greater than or equal to the first.

    Expect.atLeast -2 (List.length [])

    -- Passes because (0 >= -2) is True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because (0 >= 3) is False
    List.length []
        |> Expect.atLeast 3

    {-

    0
    ╷
    │ Expect.atLeast
    ╵
    3

    -}

-}
atLeast : comparable -> comparable -> Expectation
atLeast =
    compareWith "Expect.atLeast" (>=)


{-| A type to describe how close a floating point number must be to the expected value for the test to pass. This may be
specified as absolute or relative.

`AbsoluteOrRelative` tolerance uses a logical OR between the absolute (specified first) and relative tolerance. If you
want a logical AND, use [`Expect.all`](#all).

-}
type FloatingPointTolerance
    = Absolute Float
    | Relative Float
    | AbsoluteOrRelative Float Float


{-| Passes if the second and third arguments are equal within a tolerance
specified by the first argument. This is intended to avoid failing because of
minor inaccuracies introduced by floating point arithmetic.

    -- Fails because 0.1 + 0.2 == 0.30000000000000004 (0.1 is non-terminating in base 2)
    0.1 + 0.2 |> Expect.equal 0.3

    -- So instead write this test, which passes
    0.1 + 0.2 |> Expect.within (Absolute 0.000000001) 0.3

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because 3.14 is not close enough to pi
    3.14 |> Expect.within (Absolute 0.0001) pi

    {-

    3.14
    ╷
    │ Expect.within Absolute 0.0001
    ╵
    3.141592653589793

    -}

-}
within : FloatingPointTolerance -> Float -> Float -> Expectation
within tolerance lower upper =
    nonNegativeToleranceError tolerance "within" <|
        compareWith ("Expect.within " ++ Internal.toString tolerance)
            (withinCompare tolerance)
            lower
            upper


{-| Passes if (and only if) a call to `within` with the same arguments would have failed.
-}
notWithin : FloatingPointTolerance -> Float -> Float -> Expectation
notWithin tolerance lower upper =
    nonNegativeToleranceError tolerance "notWithin" <|
        compareWith ("Expect.notWithin " ++ Internal.toString tolerance)
            (\a b -> not <| withinCompare tolerance a b)
            lower
            upper


{-| Passes if the argument is 'True', and otherwise fails with the given message.

    Expect.true "Expected the list to be empty." (List.isEmpty [])

    -- Passes because (List.isEmpty []) is True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because List.isEmpty returns False, but we expect True.
    List.isEmpty [ 42 ]
        |> Expect.true "Expected the list to be empty."

    {-

    Expected the list to be empty.

    -}

-}
true : String -> Bool -> Expectation
true message bool =
    if bool then
        pass

    else
        fail message


{-| Passes if the argument is 'False', and otherwise fails with the given message.

    Expect.false "Expected the list not to be empty." (List.isEmpty [ 42 ])

    -- Passes because (List.isEmpty [ 42 ]) is False

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because (List.isEmpty []) is True
    List.isEmpty []
        |> Expect.false "Expected the list not to be empty."

    {-

    Expected the list not to be empty.

    -}

-}
false : String -> Bool -> Expectation
false message bool =
    if bool then
        fail message

    else
        pass


{-| Passes if the
[`Result`](https://package.elm-lang.org/packages/lang/core/latest/Result) is
an `Ok` rather than `Err`. This is useful for tests where you expect not to see
an error, but you don't care what the actual result is.

_(Tip: If your function returns a `Maybe` instead, consider `Expect.notEqual Nothing`.)_

    -- Passes
    String.toInt "not an int"
        |> Expect.err

Test failures will be printed with the unexpected `Ok` value contrasting with
any `Err`.

    -- Fails
    String.toInt "20"
        |> Expect.err

    {-

    Ok 20
    ╷
    │ Expect.err
    ╵
    Err _

    -}

-}
ok : Result a b -> Expectation
ok result =
    case result of
        Ok _ ->
            pass

        Err _ ->
            { description = "Expect.ok"
            , reason = Comparison "Ok _" (Internal.toString result)
            }
                |> Test.Expectation.fail


{-| Passes if the
[`Result`](http://package.elm-lang.org/packages/elm-lang/core/latest/Result) is
an `Err` rather than `Ok`. This is useful for tests where you expect to get an
error but you don't care what the actual error is.

_(Tip: If your function returns a `Maybe` instead, consider `Expect.equal Nothing`.)_

    -- Passes
    String.toInt "not an int"
        |> Expect.err

Test failures will be printed with the unexpected `Ok` value contrasting with
any `Err`.

    -- Fails
    String.toInt "20"
        |> Expect.err

    {-

    Ok 20
    ╷
    │ Expect.err
    ╵
    Err _

    -}

-}
err : Result a b -> Expectation
err result =
    case result of
        Ok _ ->
            { description = "Expect.err"
            , reason = Comparison "Err _" (Internal.toString result)
            }
                |> Test.Expectation.fail

        Err _ ->
            pass


{-| Passes if the arguments are equal lists.

    -- Passes
    [1, 2, 3]
        |> Expect.equalLists [1, 2, 3]

Failures resemble code written in pipeline style, so you can tell
which argument is which, and reports which index the lists first
differed at or which list was longer:

    -- Fails
    [ 1, 2, 4, 6 ]
        |> Expect.equalLists [ 1, 2, 5 ]

    {-

    [1,2,4,6]
    first diff at index index 2: +`4`, -`5`
    ╷
    │ Expect.equalLists
    ╵
    first diff at index index 2: +`5`, -`4`
    [1,2,5]

    -}

-}
equalLists : List a -> List a -> Expectation
equalLists expected actual =
    if expected == actual then
        pass

    else
        { description = "Expect.equalLists"
        , reason = ListDiff (List.map Internal.toString expected) (List.map Internal.toString actual)
        }
            |> Test.Expectation.fail


{-| Passes if the arguments are equal dicts.

    -- Passes
    (Dict.fromList [ ( 1, "one" ), ( 2, "two" ) ])
        |> Expect.equalDicts (Dict.fromList [ ( 1, "one" ), ( 2, "two" ) ])

Failures resemble code written in pipeline style, so you can tell
which argument is which, and reports which keys were missing from
or added to each dict:

    -- Fails
    (Dict.fromList [ ( 1, "one" ), ( 2, "too" ) ])
        |> Expect.equalDicts (Dict.fromList [ ( 1, "one" ), ( 2, "two" ), ( 3, "three" ) ])

    {-

    Dict.fromList [(1,"one"),(2,"too")]
    diff: -[ (2,"two"), (3,"three") ] +[ (2,"too") ]
    ╷
    │ Expect.equalDicts
    ╵
    diff: +[ (2,"two"), (3,"three") ] -[ (2,"too") ]
    Dict.fromList [(1,"one"),(2,"two"),(3,"three")]

    -}

-}
equalDicts : Dict comparable a -> Dict comparable a -> Expectation
equalDicts expected actual =
    if Dict.toList expected == Dict.toList actual then
        pass

    else
        let
            differ dict k v diffs =
                if Dict.get k dict == Just v then
                    diffs

                else
                    ( k, v ) :: diffs

            missingKeys =
                Dict.foldr (differ actual) [] expected

            extraKeys =
                Dict.foldr (differ expected) [] actual
        in
        reportCollectionFailure "Expect.equalDicts" expected actual missingKeys extraKeys


{-| Passes if the arguments are equal sets.

    -- Passes
    (Set.fromList [1, 2])
        |> Expect.equalSets (Set.fromList [1, 2])

Failures resemble code written in pipeline style, so you can tell
which argument is which, and reports which keys were missing from
or added to each set:

    -- Fails
    (Set.fromList [ 1, 2, 4, 6 ])
        |> Expect.equalSets (Set.fromList [ 1, 2, 5 ])

    {-

    Set.fromList [1,2,4,6]
    diff: -[ 5 ] +[ 4, 6 ]
    ╷
    │ Expect.equalSets
    ╵
    diff: +[ 5 ] -[ 4, 6 ]
    Set.fromList [1,2,5]

    -}

-}
equalSets : Set comparable -> Set comparable -> Expectation
equalSets expected actual =
    if Set.toList expected == Set.toList actual then
        pass

    else
        let
            missingKeys =
                Set.diff expected actual
                    |> Set.toList

            extraKeys =
                Set.diff actual expected
                    |> Set.toList
        in
        reportCollectionFailure "Expect.equalSets" expected actual missingKeys extraKeys


{-| Always passes.

    import Json.Decode exposing (decodeString, int)
    import Test exposing (test)
    import Expect


    test "Json.Decode.int can decode the number 42." <|
        \_ ->
            case decodeString int "42" of
                Ok _ ->
                    Expect.pass

                Err err ->
                    Expect.fail err

-}
pass : Expectation
pass =
    Test.Expectation.Pass


{-| Fails with the given message.

    import Json.Decode exposing (decodeString, int)
    import Test exposing (test)
    import Expect


    test "Json.Decode.int can decode the number 42." <|
        \_ ->
            case decodeString int "42" of
                Ok _ ->
                    Expect.pass

                Err err ->
                    Expect.fail err

-}
fail : String -> Expectation
fail str =
    Test.Expectation.fail { description = str, reason = Custom }


{-| If the given expectation fails, replace its failure message with a custom one.

    "something"
        |> Expect.equal "something else"
        |> Expect.onFail "thought those two strings would be the same"

-}
onFail : String -> Expectation -> Expectation
onFail str expectation =
    case expectation of
        Test.Expectation.Pass ->
            expectation

        Test.Expectation.Fail failure ->
            Test.Expectation.Fail { failure | description = str, reason = Custom }


{-| Passes if each of the given functions passes when applied to the subject.

Passing an empty list is assumed to be a mistake, so `Expect.all []`
will always return a failed expectation no matter what else it is passed.

    Expect.all
        [ Expect.greaterThan -2
        , Expect.lessThan 5
        ]
        (List.length [])
    -- Passes because (0 > -2) is True and (0 < 5) is also True

Failures resemble code written in pipeline style, so you can tell
which argument is which:

    -- Fails because (0 < -10) is False
    List.length []
        |> Expect.all
            [ Expect.greaterThan -2
            , Expect.lessThan -10
            , Expect.equal 0
            ]
    {-
    0
    ╷
    │ Expect.lessThan
    ╵
    -10
    -}

-}
all : List (subject -> Expectation) -> subject -> Expectation
all list query =
    if List.isEmpty list then
        Test.Expectation.fail
            { reason = Invalid EmptyList
            , description = "Expect.all was given an empty list. You must make at least one expectation to have a valid test!"
            }

    else
        allHelp list query


allHelp : List (subject -> Expectation) -> subject -> Expectation
allHelp list query =
    case list of
        [] ->
            pass

        check :: rest ->
            case check query of
                Test.Expectation.Pass ->
                    allHelp rest query

                outcome ->
                    outcome



{---- Private helper functions ----}


reportFailure : String -> String -> String -> Expectation
reportFailure comparison expected actual =
    { description = comparison

    -- We may need to wrap expected and actual in quotes to maintain 0.18 behavior
    , reason = Comparison expected actual
    }
        |> Test.Expectation.fail


reportCollectionFailure : String -> a -> b -> List c -> List d -> Expectation
reportCollectionFailure comparison expected actual missingKeys extraKeys =
    { description = comparison
    , reason =
        { expected = Internal.toString expected
        , actual = Internal.toString actual
        , extra = List.map Internal.toString extraKeys
        , missing = List.map Internal.toString missingKeys
        }
            |> CollectionDiff
    }
        |> Test.Expectation.fail


{-| String arg is label, e.g. "Expect.equal".
-}
equateWith : String -> (a -> b -> Bool) -> b -> a -> Expectation
equateWith reason comparison b a =
    let
        isJust x =
            case x of
                Just _ ->
                    True

                Nothing ->
                    False

        isFloat x =
            isJust (String.toFloat x) && not (isJust (String.toInt x))

        usesFloats =
            isFloat (Internal.toString a) || isFloat (Internal.toString b)

        floatError =
            if String.contains reason "not" then
                "Do not use Expect.notEqual with floats. Use Float.notWithin instead."

            else
                "Do not use Expect.equal with floats. Use Float.within instead."
    in
    if usesFloats then
        fail floatError

    else
        testWith Equality reason comparison b a


compareWith : String -> (a -> b -> Bool) -> b -> a -> Expectation
compareWith =
    testWith Comparison


testWith : (String -> String -> Reason) -> String -> (a -> b -> Bool) -> b -> a -> Expectation
testWith makeReason label runTest expected actual =
    if runTest actual expected then
        pass

    else
        { description = label
        , reason = makeReason (Internal.toString expected) (Internal.toString actual)
        }
            |> Test.Expectation.fail



{---- Private *floating point* helper functions ----}


absolute : FloatingPointTolerance -> Float
absolute tolerance =
    case tolerance of
        Absolute val ->
            val

        AbsoluteOrRelative val _ ->
            val

        _ ->
            0


relative : FloatingPointTolerance -> Float
relative tolerance =
    case tolerance of
        Relative val ->
            val

        AbsoluteOrRelative _ val ->
            val

        _ ->
            0


nonNegativeToleranceError : FloatingPointTolerance -> String -> Expectation -> Expectation
nonNegativeToleranceError tolerance name result =
    if absolute tolerance < 0 && relative tolerance < 0 then
        Test.Expectation.fail { description = "Expect." ++ name ++ " was given negative absolute and relative tolerances", reason = Custom }

    else if absolute tolerance < 0 then
        Test.Expectation.fail { description = "Expect." ++ name ++ " was given a negative absolute tolerance", reason = Custom }

    else if relative tolerance < 0 then
        Test.Expectation.fail { description = "Expect." ++ name ++ " was given a negative relative tolerance", reason = Custom }

    else
        result


withinCompare : FloatingPointTolerance -> Float -> Float -> Bool
withinCompare tolerance a b =
    let
        withinAbsoluteTolerance =
            a - absolute tolerance <= b && b <= a + absolute tolerance

        withinRelativeTolerance =
            (a - abs (a * relative tolerance) <= b && b <= a + abs (a * relative tolerance))
                || (b - abs (b * relative tolerance) <= a && a <= b + abs (b * relative tolerance))
    in
    (a == b) || withinAbsoluteTolerance || withinRelativeTolerance
