module Tuple exposing
  ( pair
  , first, second
  , mapFirst, mapSecond, mapBoth
  )

{-| Elm has built-in syntax for tuples, so you can define 2D points like this:

    origin : (Float, Float)
    origin =
      (0, 0)

    position : (Float, Float)
    position =
      (3, 4)

This module is a bunch of helpers for working with 2-tuples.

**Note 1:** For more complex data, it is best to switch to records. So instead
of representing a 3D point as `(3,4,5)` and not having any helper functions,
represent it as `{ x = 3, y = 4, z = 5 }` and use all the built-in record
syntax!

**Note 2:** If your record contains a bunch of `Bool` and `Maybe` values,
you may want to upgrade to union types. Check out [Joël’s post][ut] for more
info on this. (Picking appropriate data structures is super important in Elm!)

[ut]: https://robots.thoughtbot.com/modeling-with-union-types

# Create
@docs pair

# Access
@docs first, second

# Map
@docs mapFirst, mapSecond, mapBoth

-}



-- CREATE


{-| Create a 2-tuple.

    -- pair 3 4 == (3, 4)

    zip : List a -> List b -> List (a, b)
    zip xs ys =
      List.map2 Tuple.pair xs ys
-}
pair : a -> b -> (a, b)
pair a b =
  (a, b)



-- ACCESS


{-| Extract the first value from a tuple.

    first (3, 4) == 3
    first ("john", "doe") == "john"
-}
first : (a, b) -> a
first (x,_) =
  x


{-| Extract the second value from a tuple.

    second (3, 4) == 4
    second ("john", "doe") == "doe"
-}
second : (a, b) -> b
second (_,y) =
  y



-- MAP


{-| Transform the first value in a tuple.

    import String

    mapFirst String.reverse ("stressed", 16) == ("desserts", 16)
    mapFirst String.length  ("stressed", 16) == (8, 16)
-}
mapFirst : (a -> x) -> (a, b) -> (x, b)
mapFirst func (x,y) =
  (func x, y)


{-| Transform the second value in a tuple.

    mapSecond sqrt   ("stressed", 16) == ("stressed", 4)
    mapSecond negate ("stressed", 16) == ("stressed", -16)
-}
mapSecond : (b -> y) -> (a, b) -> (a, y)
mapSecond func (x,y) =
  (x, func y)


{-| Transform both parts of a tuple.

    import String

    mapBoth String.reverse sqrt  ("stressed", 16) == ("desserts", 4)
    mapBoth String.length negate ("stressed", 16) == (8, -16)
-}
mapBoth : (a -> x) -> (b -> y) -> (a, b) -> (x, y)
mapBoth funcA funcB (x,y) =
  ( funcA x, funcB y )
