module RoseTree exposing (..)

{-| RoseTree implementation in Elm using Lazy Lists.

This implementation is private to elm-test and has non-essential functions removed.
If you need a complete RoseTree implementation, one can be found on elm-package.

-}

import Lazy.List as LazyList exposing (LazyList, append, cons)


{-| RoseTree type.
A rosetree is a tree with a root whose children are themselves
rosetrees.
-}
type RoseTree a
    = Rose a (LazyList (RoseTree a))


{-| Make a singleton rosetree
-}
singleton : a -> RoseTree a
singleton a =
    Rose a LazyList.empty


{-| Get the root of a rosetree
-}
root : RoseTree a -> a
root (Rose a _) =
    a


{-| Get the children of a rosetree
-}
children : RoseTree a -> LazyList (RoseTree a)
children (Rose _ c) =
    c


{-| Add a child to the rosetree.
-}
addChild : RoseTree a -> RoseTree a -> RoseTree a
addChild child (Rose a c) =
    Rose a (cons child c)


{-| Map a function over a rosetree
-}
map : (a -> b) -> RoseTree a -> RoseTree b
map f (Rose a c) =
    Rose (f a) (LazyList.map (map f) c)


filter : (a -> Bool) -> RoseTree a -> Maybe (RoseTree a)
filter predicate tree =
    let
        maybeKeep x =
            if predicate x then
                Just x

            else
                Nothing
    in
    filterMap maybeKeep tree


{-| filterMap a function over a rosetree
-}
filterMap : (a -> Maybe b) -> RoseTree a -> Maybe (RoseTree b)
filterMap f (Rose a c) =
    case f a of
        Just newA ->
            Just <| Rose newA (LazyList.filterMap (filterMap f) c)

        Nothing ->
            Nothing


filterBranches : (a -> Bool) -> RoseTree a -> RoseTree a
filterBranches predicate (Rose root_ branches) =
    Rose
        root_
        (LazyList.filterMap (filter predicate) branches)


{-| Flatten a rosetree of rosetrees.
-}
flatten : RoseTree (RoseTree a) -> RoseTree a
flatten (Rose (Rose a c) cs) =
    Rose a (append c (LazyList.map flatten cs))
