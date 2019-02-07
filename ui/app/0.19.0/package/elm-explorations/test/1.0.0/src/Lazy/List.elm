module Lazy.List exposing (..)

{-| Lazy list implementation in Elm.


# Types

@docs LazyList, LazyListView


# Constructors

@docs cons, empty, singleton


# Query operations

@docs isEmpty, head, tail, headAndTail, member, length


# Conversions

@docs toList, fromList, toArray, fromArray


# Map-reduce et al.

@docs map, zip, reduce, flatten, append, foldl, foldr


# Common operations

@docs intersperse, interleave, reverse, cycle, iterate, repeat, take, takeWhile, drop, dropWhile


# Filtering operations

@docs keepIf, dropIf, filterMap, unique


# Chaining operations

@docs andMap, andThen


# Useful math stuff

@docs numbers, sum, product


# All the maps!

@docs map2, map3, map4, map5


# All the Cartesian products!

**Warning:** Calling these functions on large lists and then calling `toList` can easily overflow the stack. Consider
passing the results to `take aConstantNumber`.

@docs product2, product3

-}

import Array exposing (Array)
import Lazy exposing (Lazy, force, lazy)
import List
import Random exposing (Generator, Seed)


{-| Analogous to `List` type. This is the actual implementation type for the
`LazyList` type. This type is exposed to the user if the user so wishes to
do pattern matching or understand how the list type works. It is not
recommended to work with this type directly. Try working solely with the
provided functions in the package.
-}
type LazyListView a
    = Nil
    | Cons a (LazyList a)


{-| Lazy List type.
-}
type alias LazyList a =
    Lazy (LazyListView a)


{-| Create an empty list.
-}
empty : LazyList a
empty =
    lazy <|
        \() -> Nil


{-| Create a singleton list.
-}
singleton : a -> LazyList a
singleton a =
    cons a empty


{-| Detect if a list is empty or not.
-}
isEmpty : LazyList a -> Bool
isEmpty list =
    case force list of
        Nil ->
            True

        _ ->
            False


{-| Add a value to the front of a list.
-}
cons : a -> LazyList a -> LazyList a
cons a list =
    lazy <|
        \() ->
            Cons a list


{-| Get the head of a list.
-}
head : LazyList a -> Maybe a
head list =
    case force list of
        Nil ->
            Nothing

        Cons first _ ->
            Just first


{-| Get the tail of a list.
-}
tail : LazyList a -> Maybe (LazyList a)
tail list =
    case force list of
        Nil ->
            Nothing

        Cons _ rest ->
            Just rest


{-| Get the head and tail of a list.
-}
headAndTail : LazyList a -> Maybe ( a, LazyList a )
headAndTail list =
    case force list of
        Nil ->
            Nothing

        Cons first rest ->
            Just ( first, rest )


{-| Repeat a value ad infinitum.
Be careful when you use this. The result of this is a truly infinite list.
Do not try calling `reduce` or `toList` on an infinite list as it'll never
finish computing. Make sure you then filter it down to a finite list with `head`
or `take` or something.
-}
repeat : a -> LazyList a
repeat a =
    lazy <|
        \() ->
            Cons a (repeat a)


{-| Append a list to another list.
-}
append : LazyList a -> LazyList a -> LazyList a
append list1 list2 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    force list2

                Cons first rest ->
                    force (append (cons first rest) list2)


{-| Interleave the elements of a list in another list. The two lists get
interleaved at the end.
-}
interleave : LazyList a -> LazyList a -> LazyList a
interleave list1 list2 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    force list2

                Cons first1 rest1 ->
                    case force list2 of
                        Nil ->
                            force list1

                        Cons first2 rest2 ->
                            force (cons first1 (cons first2 (interleave rest1 rest2)))


{-| Places the given value between all members of the given list.
-}
intersperse : a -> LazyList a -> LazyList a
intersperse a list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    case force rest of
                        Nil ->
                            force (cons first empty)

                        Cons second secondRest ->
                            case force secondRest of
                                Nil ->
                                    force (cons first (cons a (cons second empty)))

                                _ ->
                                    force (cons first (cons a (cons second (cons a (intersperse a secondRest)))))


{-| Take a list and repeat it ad infinitum. This cycles a finite list
by putting the front after the end of the list. This results in a no-op in
the case of an infinite list.
-}
cycle : LazyList a -> LazyList a
cycle list =
    append list
        (lazy <|
            \() ->
                force (cycle list)
        )


{-| Create an infinite list of applications of a function on some value.

Equivalent to:

    x ::: f x ::: f (f x) ::: f (f (f x)) ::: ... -- etc...

-}
iterate : (a -> a) -> a -> LazyList a
iterate f a =
    lazy <|
        \() ->
            Cons a (iterate f (f a))


{-| The infinite list of counting numbers.

i.e.:

    1 ::: 2 ::: 3 ::: 4 ::: 5 ::: ... -- etc...

-}
numbers : LazyList number
numbers =
    iterate ((+) 1) 1


{-| Take at most `n` many values from a list.
-}
take : Int -> LazyList a -> LazyList a
take n list =
    lazy <|
        \() ->
            if n <= 0 then
                Nil
            else
                case force list of
                    Nil ->
                        Nil

                    Cons first rest ->
                        Cons first (take (n - 1) rest)


{-| Take elements from a list as long as the predicate is satisfied.
-}
takeWhile : (a -> Bool) -> LazyList a -> LazyList a
takeWhile predicate list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    if predicate first then
                        Cons first (takeWhile predicate rest)
                    else
                        Nil


{-| Drop at most `n` many values from a list.
-}
drop : Int -> LazyList a -> LazyList a
drop n list =
    lazy <|
        \() ->
            if n <= 0 then
                force list
            else
                case force list of
                    Nil ->
                        Nil

                    Cons first rest ->
                        force (drop (n - 1) rest)


{-| Drop elements from a list as long as the predicate is satisfied.
-}
dropWhile : (a -> Bool) -> LazyList a -> LazyList a
dropWhile predicate list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    if predicate first then
                        force (dropWhile predicate rest)
                    else
                        force list


{-| Test if a value is a member of a list.
-}
member : a -> LazyList a -> Bool
member a list =
    case force list of
        Nil ->
            False

        Cons first rest ->
            first == a || member a rest


{-| Get the length of a lazy list.

Warning: This will not terminate if the list is infinite.

-}
length : LazyList a -> Int
length =
    reduce (\_ n -> n + 1) 0


{-| Remove all duplicates from a list and return a list of distinct elements.
-}
unique : LazyList a -> LazyList a
unique list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    if member first rest then
                        force (unique rest)
                    else
                        Cons first (unique rest)


{-| Keep all elements in a list that satisfy the given predicate.
-}
keepIf : (a -> Bool) -> LazyList a -> LazyList a
keepIf predicate list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    if predicate first then
                        Cons first (keepIf predicate rest)
                    else
                        force (keepIf predicate rest)


{-| Drop all elements in a list that satisfy the given predicate.
-}
dropIf : (a -> Bool) -> LazyList a -> LazyList a
dropIf predicate =
    keepIf (\n -> not (predicate n))


{-| Map a function that may fail over a lazy list, keeping only
the values that were successfully transformed.
-}
filterMap : (a -> Maybe b) -> LazyList a -> LazyList b
filterMap transform list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    case transform first of
                        Just val ->
                            Cons val (filterMap transform rest)

                        Nothing ->
                            force (filterMap transform rest)


{-| Reduce a list with a given reducer and an initial value.

Example :
reduce (+) 0 (1 ::: 2 ::: 3 ::: 4 ::: empty) == 10

-}
reduce : (a -> b -> b) -> b -> LazyList a -> b
reduce reducer b list =
    case force list of
        Nil ->
            b

        Cons first rest ->
            reduce reducer (reducer first b) rest


{-| Analogous to `List.foldl`. Is an alias for `reduce`.
-}
foldl : (a -> b -> b) -> b -> LazyList a -> b
foldl =
    reduce


{-| Analogous to `List.foldr`.
-}
foldr : (a -> b -> b) -> b -> LazyList a -> b
foldr reducer b list =
    Array.foldr reducer b (toArray list)


{-| Get the sum of a list of numbers.
-}
sum : LazyList number -> number
sum =
    reduce (+) 0


{-| Get the product of a list of numbers.
-}
product : LazyList number -> number
product =
    reduce (*) 1


{-| Flatten a list of lists into a single list by appending all the inner
lists into one big list.
-}
flatten : LazyList (LazyList a) -> LazyList a
flatten list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    force (append first (flatten rest))


{-| Chain list producing operations. Map then flatten.
-}
andThen : (a -> LazyList b) -> LazyList a -> LazyList b
andThen f list =
    map f list |> flatten


{-| Reverse a list.
-}
reverse : LazyList a -> LazyList a
reverse =
    reduce cons empty


{-| Map a function to a list.
-}
map : (a -> b) -> LazyList a -> LazyList b
map f list =
    lazy <|
        \() ->
            case force list of
                Nil ->
                    Nil

                Cons first rest ->
                    Cons (f first) (map f rest)


{-| -}
map2 : (a -> b -> c) -> LazyList a -> LazyList b -> LazyList c
map2 f list1 list2 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    Nil

                Cons first1 rest1 ->
                    case force list2 of
                        Nil ->
                            Nil

                        Cons first2 rest2 ->
                            Cons (f first1 first2) (map2 f rest1 rest2)


{-| Known as `mapN` in some circles. Allows you to apply `map` in cases
where then number of arguments are greater than 5.

The argument order is such that it works well with `|>` chains.

-}
andMap : LazyList a -> LazyList (a -> b) -> LazyList b
andMap listVal listFuncs =
    map2 (<|) listFuncs listVal


{-| -}
map3 : (a -> b -> c -> d) -> LazyList a -> LazyList b -> LazyList c -> LazyList d
map3 f list1 list2 list3 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    Nil

                Cons first1 rest1 ->
                    case force list2 of
                        Nil ->
                            Nil

                        Cons first2 rest2 ->
                            case force list3 of
                                Nil ->
                                    Nil

                                Cons first3 rest3 ->
                                    Cons (f first1 first2 first3) (map3 f rest1 rest2 rest3)


{-| -}
map4 : (a -> b -> c -> d -> e) -> LazyList a -> LazyList b -> LazyList c -> LazyList d -> LazyList e
map4 f list1 list2 list3 list4 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    Nil

                Cons first1 rest1 ->
                    case force list2 of
                        Nil ->
                            Nil

                        Cons first2 rest2 ->
                            case force list3 of
                                Nil ->
                                    Nil

                                Cons first3 rest3 ->
                                    case force list4 of
                                        Nil ->
                                            Nil

                                        Cons first4 rest4 ->
                                            Cons (f first1 first2 first3 first4) (map4 f rest1 rest2 rest3 rest4)


{-| -}
map5 : (a -> b -> c -> d -> e -> f) -> LazyList a -> LazyList b -> LazyList c -> LazyList d -> LazyList e -> LazyList f
map5 f list1 list2 list3 list4 list5 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    Nil

                Cons first1 rest1 ->
                    case force list2 of
                        Nil ->
                            Nil

                        Cons first2 rest2 ->
                            case force list3 of
                                Nil ->
                                    Nil

                                Cons first3 rest3 ->
                                    case force list4 of
                                        Nil ->
                                            Nil

                                        Cons first4 rest4 ->
                                            case force list5 of
                                                Nil ->
                                                    Nil

                                                Cons first5 rest5 ->
                                                    Cons
                                                        (f first1 first2 first3 first4 first5)
                                                        (map5 f rest1 rest2 rest3 rest4 rest5)


{-| -}
zip : LazyList a -> LazyList b -> LazyList ( a, b )
zip =
    map2 Tuple.pair


{-| Create a lazy list containing all possible pairs in the given lazy lists.
-}
product2 : LazyList a -> LazyList b -> LazyList ( a, b )
product2 list1 list2 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    Nil

                Cons first1 rest1 ->
                    case force list2 of
                        Nil ->
                            Nil

                        Cons _ _ ->
                            force <| append (map (Tuple.pair first1) list2) (product2 rest1 list2)


{-| Create a lazy list containing all possible triples in the given lazy lists.
-}
product3 : LazyList a -> LazyList b -> LazyList c -> LazyList ( a, b, c )
product3 list1 list2 list3 =
    lazy <|
        \() ->
            case force list1 of
                Nil ->
                    Nil

                Cons first1 rest1 ->
                    force <| append (map (\( b, c ) -> ( first1, b, c )) (product2 list2 list3)) (product3 rest1 list2 list3)


{-| Convert a lazy list to a normal list.
-}
toList : LazyList a -> List a
toList list =
    case force list of
        Nil ->
            []

        Cons first rest ->
            first :: toList rest


{-| Convert a normal list to a lazy list.
-}
fromList : List a -> LazyList a
fromList =
    List.foldr cons empty


{-| Convert a lazy list to an array.
-}
toArray : LazyList a -> Array a
toArray list =
    case force list of
        Nil ->
            Array.empty

        Cons first rest ->
            Array.append (Array.push first Array.empty) (toArray rest)


{-| Convert an array to a lazy list.
-}
fromArray : Array a -> LazyList a
fromArray =
    Array.foldr cons empty



---------------------
-- INFIX OPERATORS --
---------------------
