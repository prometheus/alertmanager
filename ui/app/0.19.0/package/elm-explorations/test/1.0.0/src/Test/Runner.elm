module Test.Runner
    exposing
        ( Runner
        , SeededRunners(..)
        , Shrinkable
        , formatLabels
        , fromTest
        , fuzz
        , getFailureReason
        , isTodo
        , shrink
        )

{-| This is an "experts only" module that exposes functions needed to run and
display tests. A typical user will use an existing runner library for Node or
the browser, which is implemented using this interface. A list of these runners
can be found in the `README`.


## Runner

@docs Runner, SeededRunners, fromTest


## Expectations

@docs getFailureReason, isTodo


## Formatting

@docs formatLabels


## Fuzzers

These functions give you the ability to run fuzzers separate of running fuzz tests.

@docs Shrinkable, fuzz, shrink

-}

import Bitwise
import Char
import Elm.Kernel.Test
import Expect exposing (Expectation)
import Fuzz exposing (Fuzzer)
import Lazy.List as LazyList exposing (LazyList)
import Random
import RoseTree exposing (RoseTree(..))
import String
import Test exposing (Test)
import Test.Expectation
import Test.Internal as Internal
import Test.Runner.Failure exposing (Reason(..))


{-| An unevaluated test. Run it with [`run`](#run) to evaluate it into a
list of `Expectation`s.
-}
type Runnable
    = Thunk (() -> List Expectation)


{-| A function which, when evaluated, produces a list of expectations. Also a
list of labels which apply to this outcome.
-}
type alias Runner =
    { run : () -> List Expectation
    , labels : List String
    }


{-| A structured test runner, incorporating:

  - The expectations to run
  - The hierarchy of description strings that describe the results

-}
type RunnableTree
    = Runnable Runnable
    | Labeled String RunnableTree
    | Batch (List RunnableTree)


{-| Convert a `Test` into `SeededRunners`.

In order to run any fuzz tests that the `Test` may have, it requires a default run count as well
as an initial `Random.Seed`. `100` is a good run count. To obtain a good random seed, pass a
random 32-bit integer to `Random.initialSeed`. You can obtain such an integer by running
`Math.floor(Math.random()*0xFFFFFFFF)` in Node. It's typically fine to hard-code this value into
your Elm code; it's easy and makes your tests reproducible.

-}
fromTest : Int -> Random.Seed -> Test -> SeededRunners
fromTest runs seed test =
    if runs < 1 then
        Invalid ("Test runner run count must be at least 1, not " ++ String.fromInt runs)

    else
        let
            distribution =
                distributeSeeds runs seed test
        in
        if List.isEmpty distribution.only then
            if countAllRunnables distribution.skipped == 0 then
                distribution.all
                    |> List.concatMap fromRunnableTree
                    |> Plain

            else
                distribution.all
                    |> List.concatMap fromRunnableTree
                    |> Skipping

        else
            distribution.only
                |> List.concatMap fromRunnableTree
                |> Only


countAllRunnables : List RunnableTree -> Int
countAllRunnables =
    List.foldl (countRunnables >> (+)) 0


countRunnables : RunnableTree -> Int
countRunnables runnable =
    case runnable of
        Runnable _ ->
            1

        Labeled _ runner ->
            countRunnables runner

        Batch runners ->
            countAllRunnables runners


run : Runnable -> List Expectation
run (Thunk fn) =
    case runThunk fn of
        Ok tests ->
            tests

        Err message ->
            [ Expect.fail ("This test failed because it threw an exception: \"" ++ message ++ "\"") ]


runThunk : (() -> a) -> Result String a
runThunk =
    Elm.Kernel.Test.runThunk


fromRunnableTree : RunnableTree -> List Runner
fromRunnableTree =
    fromRunnableTreeHelp []


fromRunnableTreeHelp : List String -> RunnableTree -> List Runner
fromRunnableTreeHelp labels runner =
    case runner of
        Runnable runnable ->
            [ { labels = labels
              , run = \_ -> run runnable
              }
            ]

        Labeled label subRunner ->
            fromRunnableTreeHelp (label :: labels) subRunner

        Batch runners ->
            List.concatMap (fromRunnableTreeHelp labels) runners


type alias Distribution =
    { seed : Random.Seed
    , only : List RunnableTree
    , all : List RunnableTree
    , skipped : List RunnableTree
    }


{-| Test Runners which have had seeds distributed to them, and which are now
either invalid or are ready to run. Seeded runners include some metadata:

  - `Invalid` runners had a problem (e.g. two sibling tests had the same description) making them un-runnable.
  - `Only` runners can be run, but `Test.only` was used somewhere, so ultimately they will lead to a failed test run even if each test that gets run passes.
  - `Skipping` runners can be run, but `Test.skip` was used somewhere, so ultimately they will lead to a failed test run even if each test that gets run passes.
  - `Plain` runners are ready to run, and have none of these issues.

-}
type SeededRunners
    = Plain (List Runner)
    | Only (List Runner)
    | Skipping (List Runner)
    | Invalid String


emptyDistribution : Random.Seed -> Distribution
emptyDistribution seed =
    { seed = seed
    , all = []
    , only = []
    , skipped = []
    }


{-| This breaks down a test into individual Runners, while assigning different
random number seeds to them. Along the way it also does a few other things:

1.  Collect any tests created with `Test.only` so later we can run only those.
2.  Collect any tests created with `Test.todo` so later we can fail the run.
3.  Validate that the run count is at least 1.

Some design notes:

1.  `only` tests and `skip` tests do not affect seed distribution. This is
    important for the case where a user runs tests, sees one failure, and decides
    to isolate it by using both `only` and providing the same seed as before. If
    `only` changes seed distribution, then that test result might not reproduce!
    This would be very frustrating, as it would mean you could reproduce the
    failure when not using `only`, but it magically disappeared as soon as you
    tried to isolate it. The same logic applies to `skip`.

2.  Theoretically this could become tail-recursive. However, the Labeled and Batch
    cases would presumably become very gnarly, and it's unclear whether there would
    be a performance benefit or penalty in the end. If some brave soul wants to
    attempt it for kicks, beware that this is not a performance optimization for
    the faint of heart. Practically speaking, it seems unlikely to be worthwhile
    unless somehow people start seeing stack overflows during seed distribution -
    which would presumably require some absurdly deeply nested `describe` calls.

-}
distributeSeeds : Int -> Random.Seed -> Test -> Distribution
distributeSeeds =
    distributeSeedsHelp False


distributeSeedsHelp : Bool -> Int -> Random.Seed -> Test -> Distribution
distributeSeedsHelp hashed runs seed test =
    case test of
        Internal.UnitTest aRun ->
            { seed = seed
            , all = [ Runnable (Thunk (\_ -> aRun ())) ]
            , only = []
            , skipped = []
            }

        Internal.FuzzTest aRun ->
            let
                ( firstSeed, nextSeed ) =
                    Random.step Random.independentSeed seed
            in
            { seed = nextSeed
            , all = [ Runnable (Thunk (\_ -> aRun firstSeed runs)) ]
            , only = []
            , skipped = []
            }

        Internal.Labeled description subTest ->
            -- This fixes https://github.com/elm-community/elm-test/issues/192
            -- The first time we hit a Labeled, we want to use the hash of
            -- that label, along with the original seed, as our starting
            -- point for distribution. Repeating this process more than
            -- once would be a waste.
            if hashed then
                let
                    next =
                        distributeSeedsHelp True runs seed subTest
                in
                { seed = next.seed
                , all = List.map (Labeled description) next.all
                , only = List.map (Labeled description) next.only
                , skipped = List.map (Labeled description) next.skipped
                }

            else
                let
                    intFromSeed =
                        -- At this point, this seed will be the original
                        -- one passed into distributeSeeds. We know this
                        -- because the only other branch that does a
                        -- Random.step on that seed is the Internal.Test
                        -- branch, and you can't have a Labeled inside a
                        -- Test, so that couldn't have come up yet.
                        seed
                            -- Convert the Seed back to an Int
                            |> Random.step (Random.int 0 Random.maxInt)
                            |> Tuple.first

                    hashedSeed =
                        description
                            -- Hash from String to Int
                            |> fnvHashString fnvInit
                            -- Incorporate the originally passed-in seed
                            |> fnvHash intFromSeed
                            -- Convert Int back to Seed
                            |> Random.initialSeed

                    next =
                        distributeSeedsHelp True runs hashedSeed subTest
                in
                -- Using seed instead of next.seed fixes https://github.com/elm-community/elm-test/issues/192
                -- by making it so that all the tests underneath this Label begin
                -- with the hashed seed, but subsequent sibling tests in this Batch
                -- get the same seed as before.
                { seed = seed
                , all = List.map (Labeled description) next.all
                , only = List.map (Labeled description) next.only
                , skipped = List.map (Labeled description) next.skipped
                }

        Internal.Skipped subTest ->
            let
                -- Go through the motions in order to obtain the seed, but then
                -- move everything to skipped.
                next =
                    distributeSeedsHelp hashed runs seed subTest
            in
            { seed = next.seed
            , all = []
            , only = []
            , skipped = next.all
            }

        Internal.Only subTest ->
            let
                next =
                    distributeSeedsHelp hashed runs seed subTest
            in
            -- `only` all the things!
            { next | only = next.all }

        Internal.Batch tests ->
            List.foldl (batchDistribute hashed runs) (emptyDistribution seed) tests


batchDistribute : Bool -> Int -> Test -> Distribution -> Distribution
batchDistribute hashed runs test prev =
    let
        next =
            distributeSeedsHelp hashed runs prev.seed test
    in
    { seed = next.seed
    , all = prev.all ++ next.all
    , only = prev.only ++ next.only
    , skipped = prev.skipped ++ next.skipped
    }


{-| FNV-1a initial hash value
-}
fnvInit : Int
fnvInit =
    2166136261


{-| FNV-1a helper for strings, using Char.toCode
-}
fnvHashString : Int -> String -> Int
fnvHashString hash str =
    str |> String.toList |> List.map Char.toCode |> List.foldl fnvHash hash


{-| FNV-1a implementation.
-}
fnvHash : Int -> Int -> Int
fnvHash a b =
    Bitwise.xor a b * 16777619 |> Bitwise.shiftRightZfBy 0


{-| Return `Nothing` if the given [`Expectation`](#Expectation) is a [`pass`](#pass).

If it is a [`fail`](#fail), return a record containing the expectation
description, the [`Reason`](#Reason) the test failed, and the given inputs if
it was a fuzz test. (If it was not a fuzz test, the record's `given` field
will be `Nothing`).

For example:

    getFailureReason (Expect.equal 1 2)
    -- Just { reason = Equal 1 2, description = "Expect.equal", given = Nothing }

    getFailureReason (Expect.equal 1 1)
    -- Nothing

-}
getFailureReason :
    Expectation
    ->
        Maybe
            { given : Maybe String
            , description : String
            , reason : Reason
            }
getFailureReason expectation =
    case expectation of
        Test.Expectation.Pass ->
            Nothing

        Test.Expectation.Fail record ->
            Just record


{-| Determine if an expectation was created by a call to `Test.todo`. Runners
may treat these tests differently in their output.
-}
isTodo : Expectation -> Bool
isTodo expectation =
    case expectation of
        Test.Expectation.Pass ->
            False

        Test.Expectation.Fail { reason } ->
            reason == TODO


{-| A standard way to format descriptions and test labels, to keep things
consistent across test runner implementations.

The HTML, Node, String, and Log runners all use this.

What it does:

  - drop any labels that are empty strings
  - format the first label differently from the others
  - reverse the resulting list

Example:

    [ "the actual test that failed"
    , "nested description failure"
    , "top-level description failure"
    ]
    |> formatLabels ((++) "↓ ") ((++) "✗ ")

    {-
    [ "↓ top-level description failure"
    , "↓ nested description failure"
    , "✗ the actual test that failed"
    ]
    -}

-}
formatLabels :
    (String -> format)
    -> (String -> format)
    -> List String
    -> List format
formatLabels formatDescription formatTest labels =
    case List.filter (not << String.isEmpty) labels of
        [] ->
            []

        test :: descriptions ->
            descriptions
                |> List.map formatDescription
                |> (::) (formatTest test)
                |> List.reverse


type alias Shrunken a =
    { down : LazyList (RoseTree a)
    , over : LazyList (RoseTree a)
    }


{-| A `Shrinkable a` is an opaque type that allows you to obtain a value of type
`a` that is smaller than the one you've previously obtained.
-}
type Shrinkable a
    = Shrinkable (Shrunken a)


{-| Given a fuzzer, return a random generator to produce a value and a
Shrinkable. The value is what a fuzz test would have received as input.
-}
fuzz : Fuzzer a -> Result String (Random.Generator ( a, Shrinkable a ))
fuzz fuzzer =
    case fuzzer of
        Ok validFuzzer ->
            validFuzzer
                |> Random.map
                    (\(Rose root children) ->
                        ( root, Shrinkable { down = children, over = LazyList.empty } )
                    )
                |> Ok

        Err reason ->
            Err <| "Cannot call `fuzz` with an invalid fuzzer: " ++ reason


{-| Given a Shrinkable, attempt to shrink the value further. Pass `False` to
indicate that the last value you've seen (from either `fuzz` or this function)
caused the test to **fail**. This will attempt to find a smaller value. Pass
`True` if the test passed. If you have already seen a failure, this will attempt
to shrink that failure in another way. In both cases, it may be impossible to
shrink the value, represented by `Nothing`.
-}
shrink : Bool -> Shrinkable a -> Maybe ( a, Shrinkable a )
shrink causedPass (Shrinkable { down, over }) =
    let
        tryNext =
            if causedPass then
                over

            else
                down
    in
    case LazyList.headAndTail tryNext of
        Just ( Rose root children, tl ) ->
            Just ( root, Shrinkable { down = children, over = tl } )

        Nothing ->
            Nothing
