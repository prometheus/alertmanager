module Maybe exposing
  ( Maybe(..)
  , andThen
  , map, map2, map3, map4, map5
  , withDefault
  )

{-| This library fills a bunch of important niches in Elm. A `Maybe` can help
you with optional arguments, error handling, and records with optional fields.

# Definition
@docs Maybe

# Common Helpers
@docs withDefault, map, map2, map3, map4, map5

# Chaining Maybes
@docs andThen
-}


import Basics exposing (Bool(..))



{-| Represent values that may or may not exist. It can be useful if you have a
record field that is only filled in sometimes. Or if a function takes a value
sometimes, but does not absolutely need it.

    -- A person, but maybe we do not know their age.
    type alias Person =
        { name : String
        , age : Maybe Int
        }

    tom = { name = "Tom", age = Just 42 }
    sue = { name = "Sue", age = Nothing }
-}
type Maybe a
    = Just a
    | Nothing


{-| Provide a default value, turning an optional value into a normal
value.  This comes in handy when paired with functions like
[`Dict.get`](Dict#get) which gives back a `Maybe`.

    withDefault 100 (Just 42)   -- 42
    withDefault 100 Nothing     -- 100

    withDefault "unknown" (Dict.get "Tom" Dict.empty)   -- "unknown"

**Note:** This can be overused! Many cases are better handled by a `case`
expression. And if you end up using `withDefault` a lot, it can be a good sign
that a [custom type][ct] will clean your code up quite a bit!

[ct]: https://guide.elm-lang.org/types/custom_types.html
-}
withDefault : a -> Maybe a -> a
withDefault default maybe =
    case maybe of
      Just value -> value
      Nothing -> default


{-| Transform a `Maybe` value with a given function:

    map sqrt (Just 9) == Just 3
    map sqrt Nothing  == Nothing

    map sqrt (String.toFloat "9") == Just 3
    map sqrt (String.toFloat "x") == Nothing

-}
map : (a -> b) -> Maybe a -> Maybe b
map f maybe =
  case maybe of
    Just value ->
      Just (f value)

    Nothing ->
      Nothing


{-| Apply a function if all the arguments are `Just` a value.

    map2 (+) (Just 3) (Just 4) == Just 7
    map2 (+) (Just 3) Nothing == Nothing
    map2 (+) Nothing (Just 4) == Nothing

    map2 (+) (String.toInt "1") (String.toInt "123") == Just 124
    map2 (+) (String.toInt "x") (String.toInt "123") == Nothing
    map2 (+) (String.toInt "1") (String.toInt "1.3") == Nothing
-}
map2 : (a -> b -> value) -> Maybe a -> Maybe b -> Maybe value
map2 func ma mb =
  case ma of
    Nothing ->
      Nothing

    Just a ->
      case mb of
        Nothing ->
          Nothing

        Just b ->
          Just (func a b)


{-|-}
map3 : (a -> b -> c -> value) -> Maybe a -> Maybe b -> Maybe c -> Maybe value
map3 func ma mb mc =
  case ma of
    Nothing ->
      Nothing

    Just a ->
      case mb of
        Nothing ->
          Nothing

        Just b ->
          case mc of
            Nothing ->
              Nothing

            Just c ->
              Just (func a b c)


{-|-}
map4 : (a -> b -> c -> d -> value) -> Maybe a -> Maybe b -> Maybe c -> Maybe d -> Maybe value
map4 func ma mb mc md =
  case ma of
    Nothing ->
      Nothing

    Just a ->
      case mb of
        Nothing ->
          Nothing

        Just b ->
          case mc of
            Nothing ->
              Nothing

            Just c ->
              case md of
                Nothing ->
                  Nothing

                Just d ->
                  Just (func a b c d)


{-|-}
map5 : (a -> b -> c -> d -> e -> value) -> Maybe a -> Maybe b -> Maybe c -> Maybe d -> Maybe e -> Maybe value
map5 func ma mb mc md me =
  case ma of
    Nothing ->
      Nothing

    Just a ->
      case mb of
        Nothing ->
          Nothing

        Just b ->
          case mc of
            Nothing ->
              Nothing

            Just c ->
              case md of
                Nothing ->
                  Nothing

                Just d ->
                  case me of
                    Nothing ->
                      Nothing

                    Just e ->
                      Just (func a b c d e)


{-| Chain together many computations that may fail. It is helpful to see its
definition:

    andThen : (a -> Maybe b) -> Maybe a -> Maybe b
    andThen callback maybe =
        case maybe of
            Just value ->
                callback value

            Nothing ->
                Nothing

This means we only continue with the callback if things are going well. For
example, say you need to parse some user input as a month:

    parseMonth : String -> Maybe Int
    parseMonth userInput =
        String.toInt userInput
          |> andThen toValidMonth

    toValidMonth : Int -> Maybe Int
    toValidMonth month =
        if 1 <= month && month <= 12 then
            Just month
        else
            Nothing

In the `parseMonth` function, if `String.toInt` produces `Nothing` (because
the `userInput` was not an integer) this entire chain of operations will
short-circuit and result in `Nothing`. If `toValidMonth` results in `Nothing`,
again the chain of computations will result in `Nothing`.
-}
andThen : (a -> Maybe b) -> Maybe a -> Maybe b
andThen callback maybeValue =
    case maybeValue of
        Just value ->
            callback value

        Nothing ->
            Nothing



-- FOR INTERNAL USE ONLY
--
-- Use `case` expressions for this in Elm code!


isJust : Maybe a -> Bool
isJust maybe =
  case maybe of
    Just _ ->
      True

    Nothing ->
      False


destruct : b -> (a -> b) -> Maybe a -> b
destruct default func maybe =
  case maybe of
    Just a ->
      func a

    Nothing ->
      default
