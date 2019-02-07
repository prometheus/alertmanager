module Test exposing (FuzzOptions, Test, concat, describe, fuzz, fuzz2, fuzz3, fuzzWith, only, skip, test, todo)

{-| A module containing functions for creating and managing tests.

@docs Test, test


## Organizing Tests

@docs describe, concat, todo, skip, only


## Fuzz Testing

@docs fuzz, fuzz2, fuzz3, fuzzWith, FuzzOptions

-}

import Expect exposing (Expectation)
import Fuzz exposing (Fuzzer)
import Set
import Test.Fuzz
import Test.Internal as Internal
import Test.Runner.Failure exposing (InvalidReason(..), Reason(..))


{-| A test which has yet to be evaluated. When evaluated, it produces one
or more [`Expectation`](../Expect#Expectation)s.

See [`test`](#test) and [`fuzz`](#fuzz) for some ways to create a `Test`.

-}
type alias Test =
    Internal.Test


{-| Run each of the given tests.

    concat [ testDecoder, testSorting ]

-}
concat : List Test -> Test
concat tests =
    if List.isEmpty tests then
        Internal.failNow
            { description = "This `concat` has no tests in it. Let's give it some!"
            , reason = Invalid EmptyList
            }

    else
        case Internal.duplicatedName tests of
            Err duped ->
                Internal.failNow
                    { description = "A test group contains multiple tests named '" ++ duped ++ "'. Do some renaming so that tests have unique names."
                    , reason = Invalid DuplicatedName
                    }

            Ok _ ->
                Internal.Batch tests


{-| Apply a description to a list of tests.

    import Test exposing (describe, test, fuzz)
    import Fuzz exposing (int)
    import Expect


    describe "List"
        [ describe "reverse"
            [ test "has no effect on an empty list" <|
                \_ ->
                    List.reverse []
                        |> Expect.equal []
            , fuzz int "has no effect on a one-item list" <|
                \num ->
                     List.reverse [ num ]
                        |> Expect.equal [ num ]
            ]
        ]

Passing an empty list will result in a failing test, because you either made a
mistake or are creating a placeholder.

-}
describe : String -> List Test -> Test
describe untrimmedDesc tests =
    let
        desc =
            String.trim untrimmedDesc
    in
    if String.isEmpty desc then
        Internal.failNow
            { description = "This `describe` has a blank description. Let's give it a useful one!"
            , reason = Invalid BadDescription
            }

    else if List.isEmpty tests then
        Internal.failNow
            { description = "This `describe " ++ desc ++ "` has no tests in it. Let's give it some!"
            , reason = Invalid EmptyList
            }

    else
        case Internal.duplicatedName tests of
            Err duped ->
                Internal.failNow
                    { description = "The tests '" ++ desc ++ "' contain multiple tests named '" ++ duped ++ "'. Let's rename them so we know which is which."
                    , reason = Invalid DuplicatedName
                    }

            Ok childrenNames ->
                if Set.member desc childrenNames then
                    Internal.failNow
                        { description = "The test '" ++ desc ++ "' contains a child test of the same name. Let's rename them so we know which is which."
                        , reason = Invalid DuplicatedName
                        }

                else
                    Internal.Labeled desc (Internal.Batch tests)


{-| Return a [`Test`](#Test) that evaluates a single
[`Expectation`](../Expect#Expectation).

    import Test exposing (fuzz)
    import Expect


    test "the empty list has 0 length" <|
        \_ ->
            List.length []
                |> Expect.equal 0

-}
test : String -> (() -> Expectation) -> Test
test untrimmedDesc thunk =
    let
        desc =
            String.trim untrimmedDesc
    in
    if String.isEmpty desc then
        Internal.blankDescriptionFailure

    else
        Internal.Labeled desc (Internal.UnitTest (\() -> [ thunk () ]))


{-| Returns a [`Test`](#Test) that is "TODO" (not yet implemented). These tests
always fail, but test runners will only include them in their output if there
are no other failures.

These tests aren't meant to be committed to version control. Instead, use them
when you're brainstorming lots of tests you'd like to write, but you can't
implement them all at once. When you replace `todo` with a real test, you'll be
able to see if it fails without clutter from tests still not implemented. But,
unlike leaving yourself comments, you'll be prompted to implement these tests
because your suite will fail.

    describe "a new thing"
        [ todo "does what is expected in the common case"
        , todo "correctly handles an edge case I just thought of"
        ]

This functionality is similar to "pending" tests in other frameworks, except
that a TODO test is considered failing but a pending test often is not.

-}
todo : String -> Test
todo desc =
    Internal.failNow
        { description = desc
        , reason = TODO
        }


{-| Returns a [`Test`](#Test) that causes other tests to be skipped, and
only runs the given one.

Calls to `only` aren't meant to be committed to version control. Instead, use
them when you want to focus on getting a particular subset of your tests to pass.
If you use `only`, your entire test suite will fail, even if
each of the individual tests pass. This is to help avoid accidentally
committing a `only` to version control.

If you you use `only` on multiple tests, only those tests will run. If you
put a `only` inside another `only`, only the outermost `only`
will affect which tests gets run.

See also [`skip`](#skip). Note that `skip` takes precedence over `only`;
if you use a `skip` inside an `only`, it will still get skipped, and if you use
an `only` inside a `skip`, it will also get skipped.

    describe "List"
        [ only <| describe "reverse"
            [ test "has no effect on an empty list" <|
                \_ ->
                    List.reverse []
                        |> Expect.equal []
            , fuzz int "has no effect on a one-item list" <|
                \num ->
                     List.reverse [ num ]
                        |> Expect.equal [ num ]
            ]
        , test "This will not get run, because of the `only` above!" <|
            \_ ->
                List.length []
                    |> Expect.equal 0
        ]

-}
only : Test -> Test
only =
    Internal.Only


{-| Returns a [`Test`](#Test) that gets skipped.

Calls to `skip` aren't meant to be committed to version control. Instead, use
it when you want to focus on getting a particular subset of your tests to
pass. If you use `skip`, your entire test suite will fail, even if
each of the individual tests pass. This is to help avoid accidentally
committing a `skip` to version control.

See also [`only`](#only). Note that `skip` takes precedence over `only`;
if you use a `skip` inside an `only`, it will still get skipped, and if you use
an `only` inside a `skip`, it will also get skipped.

    describe "List"
        [ skip <| describe "reverse"
            [ test "has no effect on an empty list" <|
                \_ ->
                    List.reverse []
                        |> Expect.equal []
            , fuzz int "has no effect on a one-item list" <|
                \num ->
                     List.reverse [ num ]
                        |> Expect.equal [ num ]
            ]
        , test "This is the only test that will get run; the other was skipped!" <|
            \_ ->
                List.length []
                    |> Expect.equal 0
        ]

-}
skip : Test -> Test
skip =
    Internal.Skipped


{-| Options [`fuzzWith`](#fuzzWith) accepts. Currently there is only one but this
API is designed so that it can accept more in the future.


### `runs`

The number of times to run each fuzz test. (Default is 100.)

    import Test exposing (fuzzWith)
    import Fuzz exposing (list, int)
    import Expect


    fuzzWith { runs = 350 } (list int) "List.length should always be positive" <|
        -- This anonymous function will be run 350 times, each time with a
        -- randomly-generated fuzzList value. (It will always be a list of ints
        -- because of (list int) above.)
        \fuzzList ->
            fuzzList
                |> List.length
                |> Expect.atLeast 0

-}
type alias FuzzOptions =
    { runs : Int }


{-| Run a [`fuzz`](#fuzz) test with the given [`FuzzOptions`](#FuzzOptions).

Note that there is no `fuzzWith2`, but you can always pass more fuzz values in
using [`Fuzz.tuple`](Fuzz#tuple), [`Fuzz.tuple3`](Fuzz#tuple3),
for example like this:

    import Test exposing (fuzzWith)
    import Fuzz exposing (tuple, list, int)
    import Expect


    fuzzWith { runs = 4200 }
        (tuple ( list int, int ))
        "List.reverse never influences List.member" <|
            \(nums, target) ->
                List.member target (List.reverse nums)
                    |> Expect.equal (List.member target nums)

-}
fuzzWith : FuzzOptions -> Fuzzer a -> String -> (a -> Expectation) -> Test
fuzzWith options fuzzer desc getTest =
    if options.runs < 1 then
        Internal.failNow
            { description = "Fuzz tests must have a run count of at least 1, not " ++ String.fromInt options.runs ++ "."
            , reason = Invalid NonpositiveFuzzCount
            }

    else
        fuzzWithHelp options (fuzz fuzzer desc getTest)


fuzzWithHelp : FuzzOptions -> Test -> Test
fuzzWithHelp options aTest =
    case aTest of
        Internal.UnitTest _ ->
            aTest

        Internal.FuzzTest run ->
            Internal.FuzzTest (\seed _ -> run seed options.runs)

        Internal.Labeled label subTest ->
            Internal.Labeled label (fuzzWithHelp options subTest)

        Internal.Skipped subTest ->
            -- It's important to treat skipped tests exactly the same as normal,
            -- until after seed distribution has completed.
            fuzzWithHelp options subTest
                |> Internal.Only

        Internal.Only subTest ->
            fuzzWithHelp options subTest
                |> Internal.Only

        Internal.Batch tests ->
            tests
                |> List.map (fuzzWithHelp options)
                |> Internal.Batch


{-| Take a function that produces a test, and calls it several (usually 100) times, using a randomly-generated input
from a [`Fuzzer`](http://package.elm-lang.org/packages/elm-community/elm-test/latest/Fuzz) each time. This allows you to
test that a property that should always be true is indeed true under a wide variety of conditions. The function also
takes a string describing the test.

These are called "[fuzz tests](https://en.wikipedia.org/wiki/Fuzz_testing)" because of the randomness.
You may find them elsewhere called [property-based tests](http://blog.jessitron.com/2013/04/property-based-testing-what-is-it.html),
[generative tests](http://www.pivotaltracker.com/community/tracker-blog/generative-testing), or
[QuickCheck-style tests](https://en.wikipedia.org/wiki/QuickCheck).

    import Test exposing (fuzz)
    import Fuzz exposing (list, int)
    import Expect


    fuzz (list int) "List.length should always be positive" <|
        -- This anonymous function will be run 100 times, each time with a
        -- randomly-generated fuzzList value.
        \fuzzList ->
            fuzzList
                |> List.length
                |> Expect.atLeast 0

-}
fuzz :
    Fuzzer a
    -> String
    -> (a -> Expectation)
    -> Test
fuzz =
    Test.Fuzz.fuzzTest


{-| Run a [fuzz test](#fuzz) using two random inputs.

This is a convenience function that lets you skip calling [`Fuzz.tuple`](Fuzz#tuple).

See [`fuzzWith`](#fuzzWith) for an example of writing this in tuple style.

    import Test exposing (fuzz2)
    import Fuzz exposing (list, int)


    fuzz2 (list int) int "List.reverse never influences List.member" <|
        \nums target ->
            List.member target (List.reverse nums)
                |> Expect.equal (List.member target nums)

-}
fuzz2 :
    Fuzzer a
    -> Fuzzer b
    -> String
    -> (a -> b -> Expectation)
    -> Test
fuzz2 fuzzA fuzzB desc =
    let
        fuzzer =
            Fuzz.tuple ( fuzzA, fuzzB )
    in
    (\f ( a, b ) -> f a b) >> fuzz fuzzer desc


{-| Run a [fuzz test](#fuzz) using three random inputs.

This is a convenience function that lets you skip calling [`Fuzz.tuple3`](Fuzz#tuple3).

-}
fuzz3 :
    Fuzzer a
    -> Fuzzer b
    -> Fuzzer c
    -> String
    -> (a -> b -> c -> Expectation)
    -> Test
fuzz3 fuzzA fuzzB fuzzC desc =
    let
        fuzzer =
            Fuzz.tuple3 ( fuzzA, fuzzB, fuzzC )
    in
    uncurry3 >> fuzz fuzzer desc



-- INTERNAL HELPERS --


uncurry3 : (a -> b -> c -> d) -> ( a, b, c ) -> d
uncurry3 fn ( a, b, c ) =
    fn a b c
