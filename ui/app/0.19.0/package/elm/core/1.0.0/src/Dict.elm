module Dict exposing
  ( Dict
  , empty, singleton, insert, update, remove
  , isEmpty, member, get, size
  , keys, values, toList, fromList
  , map, foldl, foldr, filter, partition
  , union, intersect, diff, merge
  )

{-| A dictionary mapping unique keys to values. The keys can be any comparable
type. This includes `Int`, `Float`, `Time`, `Char`, `String`, and tuples or
lists of comparable types.

Insert, remove, and query operations all take *O(log n)* time.

# Dictionaries
@docs Dict

# Build
@docs empty, singleton, insert, update, remove

# Query
@docs isEmpty, member, get, size

# Lists
@docs keys, values, toList, fromList

# Transform
@docs map, foldl, foldr, filter, partition

# Combine
@docs union, intersect, diff, merge

-}


import Basics exposing (..)
import Maybe exposing (..)
import List exposing (..)



-- DICTIONARIES


-- The color of a node. Leaves are considered Black.
type NColor
    = Red
    | Black


{-| A dictionary of keys and values. So a `Dict String User` is a dictionary
that lets you look up a `String` (such as user names) and find the associated
`User`.

    import Dict exposing (Dict)

    users : Dict String User
    users =
      Dict.fromList
        [ ("Alice", User "Alice" 28 1.65)
        , ("Bob"  , User "Bob"   19 1.82)
        , ("Chuck", User "Chuck" 33 1.75)
        ]

    type alias User =
      { name : String
      , age : Int
      , height : Float
      }
-}
type Dict k v
    = RBNode_elm_builtin NColor k v (Dict k v) (Dict k v)
    | RBEmpty_elm_builtin


{-| Create an empty dictionary. -}
empty : Dict k v
empty =
  RBEmpty_elm_builtin


{-| Get the value associated with a key. If the key is not found, return
`Nothing`. This is useful when you are not sure if a key will be in the
dictionary.

    animals = fromList [ ("Tom", Cat), ("Jerry", Mouse) ]

    get "Tom"   animals == Just Cat
    get "Jerry" animals == Just Mouse
    get "Spike" animals == Nothing

-}
get : comparable -> Dict comparable v -> Maybe v
get targetKey dict =
  case dict of
    RBEmpty_elm_builtin ->
      Nothing

    RBNode_elm_builtin _ key value left right ->
      case compare targetKey key of
        LT ->
          get targetKey left

        EQ ->
          Just value

        GT ->
          get targetKey right


{-| Determine if a key is in a dictionary. -}
member : comparable -> Dict comparable v -> Bool
member key dict =
  case get key dict of
    Just _ ->
      True

    Nothing ->
      False


{-| Determine the number of key-value pairs in the dictionary. -}
size : Dict k v -> Int
size dict =
  sizeHelp 0 dict


sizeHelp : Int -> Dict k v -> Int
sizeHelp n dict =
  case dict of
    RBEmpty_elm_builtin ->
      n

    RBNode_elm_builtin _ _ _ left right ->
      sizeHelp (sizeHelp (n+1) right) left


{-| Determine if a dictionary is empty.

    isEmpty empty == True
-}
isEmpty : Dict k v -> Bool
isEmpty dict =
  case dict of
    RBEmpty_elm_builtin ->
      True

    RBNode_elm_builtin _ _ _ _ _ ->
      False


{-| Insert a key-value pair into a dictionary. Replaces value when there is
a collision. -}
insert : comparable -> v -> Dict comparable v -> Dict comparable v
insert key value dict =
  -- Root node is always Black
  case insertHelp key value dict of
    RBNode_elm_builtin Red k v l r ->
      RBNode_elm_builtin Black k v l r

    x ->
      x


insertHelp : comparable -> v -> Dict comparable v -> Dict comparable v
insertHelp key value dict =
  case dict of
    RBEmpty_elm_builtin ->
      -- New nodes are always red. If it violates the rules, it will be fixed
      -- when balancing.
      RBNode_elm_builtin Red key value RBEmpty_elm_builtin RBEmpty_elm_builtin

    RBNode_elm_builtin nColor nKey nValue nLeft nRight ->
      case compare key nKey of
        LT ->
          balance nColor nKey nValue (insertHelp key value nLeft) nRight

        EQ ->
          RBNode_elm_builtin nColor nKey value nLeft nRight

        GT ->
          balance nColor nKey nValue nLeft (insertHelp key value nRight)


balance : NColor -> k -> v -> Dict k v -> Dict k v -> Dict k v
balance color key value left right =
  case right of
    RBNode_elm_builtin Red rK rV rLeft rRight ->
      case left of
        RBNode_elm_builtin Red lK lV lLeft lRight ->
          RBNode_elm_builtin
            Red
            key
            value
            (RBNode_elm_builtin Black lK lV lLeft lRight)
            (RBNode_elm_builtin Black rK rV rLeft rRight)

        _ ->
          RBNode_elm_builtin color rK rV (RBNode_elm_builtin Red key value left rLeft) rRight

    _ ->
      case left of
        RBNode_elm_builtin Red lK lV (RBNode_elm_builtin Red llK llV llLeft llRight) lRight ->
          RBNode_elm_builtin
            Red
            lK
            lV
            (RBNode_elm_builtin Black llK llV llLeft llRight)
            (RBNode_elm_builtin Black key value lRight right)

        _ ->
          RBNode_elm_builtin color key value left right


{-| Remove a key-value pair from a dictionary. If the key is not found,
no changes are made. -}
remove : comparable -> Dict comparable v -> Dict comparable v
remove key dict =
  -- Root node is always Black
  case removeHelp key dict of
    RBNode_elm_builtin Red k v l r ->
      RBNode_elm_builtin Black k v l r

    x ->
      x


{-| The easiest thing to remove from the tree, is a red node. However, when searching for the
node to remove, we have no way of knowing if it will be red or not. This remove implementation
makes sure that the bottom node is red by moving red colors down the tree through rotation
and color flips. Any violations this will cause, can easily be fixed by balancing on the way
up again.
-}
removeHelp : comparable -> Dict comparable v -> Dict comparable v
removeHelp targetKey dict =
  case dict of
    RBEmpty_elm_builtin ->
      RBEmpty_elm_builtin

    RBNode_elm_builtin color key value left right ->
      if targetKey < key then
        case left of
          RBNode_elm_builtin Black _ _ lLeft _ ->
            case lLeft of
              RBNode_elm_builtin Red _ _ _ _ ->
                RBNode_elm_builtin color key value (removeHelp targetKey left) right

              _ ->
                case moveRedLeft dict of
                  RBNode_elm_builtin nColor nKey nValue nLeft nRight ->
                    balance nColor nKey nValue (removeHelp targetKey nLeft) nRight

                  RBEmpty_elm_builtin ->
                    RBEmpty_elm_builtin

          _ ->
            RBNode_elm_builtin color key value (removeHelp targetKey left) right
      else
        removeHelpEQGT targetKey (removeHelpPrepEQGT targetKey dict color key value left right)


removeHelpPrepEQGT : comparable -> Dict comparable v -> NColor -> comparable -> v -> Dict comparable v -> Dict comparable v -> Dict comparable v
removeHelpPrepEQGT targetKey dict color key value left right =
  case left of
    RBNode_elm_builtin Red lK lV lLeft lRight ->
      RBNode_elm_builtin
        color
        lK
        lV
        lLeft
        (RBNode_elm_builtin Red key value lRight right)

    _ ->
      case right of
        RBNode_elm_builtin Black _ _ (RBNode_elm_builtin Black _ _ _ _) _ ->
          moveRedRight dict

        RBNode_elm_builtin Black _ _ RBEmpty_elm_builtin _ ->
          moveRedRight dict

        _ ->
          dict


{-| When we find the node we are looking for, we can remove by replacing the key-value
pair with the key-value pair of the left-most node on the right side (the closest pair).
-}
removeHelpEQGT : comparable -> Dict comparable v -> Dict comparable v
removeHelpEQGT targetKey dict =
  case dict of
    RBNode_elm_builtin color key value left right ->
      if targetKey == key then
        case getMin right of
          RBNode_elm_builtin _ minKey minValue _ _ ->
            balance color minKey minValue left (removeMin right)

          RBEmpty_elm_builtin ->
            RBEmpty_elm_builtin
      else
        balance color key value left (removeHelp targetKey right)

    RBEmpty_elm_builtin ->
      RBEmpty_elm_builtin


getMin : Dict k v -> Dict k v
getMin dict =
  case dict of
    RBNode_elm_builtin _ _ _ ((RBNode_elm_builtin _ _ _ _ _) as left) _ ->
      getMin left

    _ ->
      dict


removeMin : Dict k v -> Dict k v
removeMin dict =
  case dict of
    RBNode_elm_builtin color key value ((RBNode_elm_builtin lColor _ _ lLeft _) as left) right ->
      case lColor of
        Black ->
          case lLeft of
            RBNode_elm_builtin Red _ _ _ _ ->
              RBNode_elm_builtin color key value (removeMin left) right

            _ ->
              case moveRedLeft dict of
                RBNode_elm_builtin nColor nKey nValue nLeft nRight ->
                  balance nColor nKey nValue (removeMin nLeft) nRight

                RBEmpty_elm_builtin ->
                  RBEmpty_elm_builtin

        _ ->
          RBNode_elm_builtin color key value (removeMin left) right

    _ ->
      RBEmpty_elm_builtin


moveRedLeft : Dict k v -> Dict k v
moveRedLeft dict =
  case dict of
    RBNode_elm_builtin clr k v (RBNode_elm_builtin lClr lK lV lLeft lRight) (RBNode_elm_builtin rClr rK rV ((RBNode_elm_builtin Red rlK rlV rlL rlR) as rLeft) rRight) ->
      RBNode_elm_builtin
        Red
        rlK
        rlV
        (RBNode_elm_builtin Black k v (RBNode_elm_builtin Red lK lV lLeft lRight) rlL)
        (RBNode_elm_builtin Black rK rV rlR rRight)

    RBNode_elm_builtin clr k v (RBNode_elm_builtin lClr lK lV lLeft lRight) (RBNode_elm_builtin rClr rK rV rLeft rRight) ->
      case clr of
        Black ->
          RBNode_elm_builtin
            Black
            k
            v
            (RBNode_elm_builtin Red lK lV lLeft lRight)
            (RBNode_elm_builtin Red rK rV rLeft rRight)

        Red ->
          RBNode_elm_builtin
            Black
            k
            v
            (RBNode_elm_builtin Red lK lV lLeft lRight)
            (RBNode_elm_builtin Red rK rV rLeft rRight)

    _ ->
      dict


moveRedRight : Dict k v -> Dict k v
moveRedRight dict =
  case dict of
    RBNode_elm_builtin clr k v (RBNode_elm_builtin lClr lK lV (RBNode_elm_builtin Red llK llV llLeft llRight) lRight) (RBNode_elm_builtin rClr rK rV rLeft rRight) ->
      RBNode_elm_builtin
        Red
        lK
        lV
        (RBNode_elm_builtin Black llK llV llLeft llRight)
        (RBNode_elm_builtin Black k v lRight (RBNode_elm_builtin Red rK rV rLeft rRight))

    RBNode_elm_builtin clr k v (RBNode_elm_builtin lClr lK lV lLeft lRight) (RBNode_elm_builtin rClr rK rV rLeft rRight) ->
      case clr of
        Black ->
          RBNode_elm_builtin
            Black
            k
            v
            (RBNode_elm_builtin Red lK lV lLeft lRight)
            (RBNode_elm_builtin Red rK rV rLeft rRight)

        Red ->
          RBNode_elm_builtin
            Black
            k
            v
            (RBNode_elm_builtin Red lK lV lLeft lRight)
            (RBNode_elm_builtin Red rK rV rLeft rRight)

    _ ->
      dict


{-| Update the value of a dictionary for a specific key with a given function. -}
update : comparable -> (Maybe v -> Maybe v) -> Dict comparable v -> Dict comparable v
update targetKey alter dictionary =
  case alter (get targetKey dictionary) of
    Just value ->
      insert targetKey value dictionary

    Nothing ->
      remove targetKey dictionary


{-| Create a dictionary with one key-value pair. -}
singleton : comparable -> v -> Dict comparable v
singleton key value =
  -- Root node is always Black
  RBNode_elm_builtin Black key value RBEmpty_elm_builtin RBEmpty_elm_builtin


-- COMBINE


{-| Combine two dictionaries. If there is a collision, preference is given
to the first dictionary.
-}
union : Dict comparable v -> Dict comparable v -> Dict comparable v
union t1 t2 =
  foldl insert t2 t1


{-| Keep a key-value pair when its key appears in the second dictionary.
Preference is given to values in the first dictionary.
-}
intersect : Dict comparable v -> Dict comparable v -> Dict comparable v
intersect t1 t2 =
  filter (\k _ -> member k t2) t1


{-| Keep a key-value pair when its key does not appear in the second dictionary.
-}
diff : Dict comparable a -> Dict comparable b -> Dict comparable a
diff t1 t2 =
  foldl (\k v t -> remove k t) t1 t2


{-| The most general way of combining two dictionaries. You provide three
accumulators for when a given key appears:

  1. Only in the left dictionary.
  2. In both dictionaries.
  3. Only in the right dictionary.

You then traverse all the keys from lowest to highest, building up whatever
you want.
-}
merge
  :  (comparable -> a -> result -> result)
  -> (comparable -> a -> b -> result -> result)
  -> (comparable -> b -> result -> result)
  -> Dict comparable a
  -> Dict comparable b
  -> result
  -> result
merge leftStep bothStep rightStep leftDict rightDict initialResult =
  let
    stepState rKey rValue (list, result) =
      case list of
        [] ->
          (list, rightStep rKey rValue result)

        (lKey, lValue) :: rest ->
          if lKey < rKey then
            stepState rKey rValue (rest, leftStep lKey lValue result)

          else if lKey > rKey then
            (list, rightStep rKey rValue result)

          else
            (rest, bothStep lKey lValue rValue result)

    (leftovers, intermediateResult) =
      foldl stepState (toList leftDict, initialResult) rightDict
  in
    List.foldl (\(k,v) result -> leftStep k v result) intermediateResult leftovers



-- TRANSFORM


{-| Apply a function to all values in a dictionary.
-}
map : (k -> a -> b) -> Dict k a -> Dict k b
map func dict =
  case dict of
    RBEmpty_elm_builtin ->
      RBEmpty_elm_builtin

    RBNode_elm_builtin color key value left right ->
      RBNode_elm_builtin color key (func key value) (map func left) (map func right)


{-| Fold over the key-value pairs in a dictionary from lowest key to highest key.

    import Dict exposing (Dict)

    getAges : Dict String User -> List String
    getAges users =
      Dict.foldl addAge [] users

    addAge : String -> User -> List String -> List String
    addAge _ user ages =
      user.age :: ages

    -- getAges users == [33,19,28]
-}
foldl : (k -> v -> b -> b) -> b -> Dict k v -> b
foldl func acc dict =
  case dict of
    RBEmpty_elm_builtin ->
      acc

    RBNode_elm_builtin _ key value left right ->
      foldl func (func key value (foldl func acc left)) right


{-| Fold over the key-value pairs in a dictionary from highest key to lowest key.

    import Dict exposing (Dict)

    getAges : Dict String User -> List String
    getAges users =
      Dict.foldr addAge [] users

    addAge : String -> User -> List String -> List String
    addAge _ user ages =
      user.age :: ages

    -- getAges users == [28,19,33]
-}
foldr : (k -> v -> b -> b) -> b -> Dict k v -> b
foldr func acc t =
  case t of
    RBEmpty_elm_builtin ->
      acc

    RBNode_elm_builtin _ key value left right ->
      foldr func (func key value (foldr func acc right)) left


{-| Keep only the key-value pairs that pass the given test. -}
filter : (comparable -> v -> Bool) -> Dict comparable v -> Dict comparable v
filter isGood dict =
  foldl (\k v d -> if isGood k v then insert k v d else d) empty dict


{-| Partition a dictionary according to some test. The first dictionary
contains all key-value pairs which passed the test, and the second contains
the pairs that did not.
-}
partition : (comparable -> v -> Bool) -> Dict comparable v -> (Dict comparable v, Dict comparable v)
partition isGood dict =
  let
    add key value (t1, t2) =
      if isGood key value then
        (insert key value t1, t2)

      else
        (t1, insert key value t2)
  in
    foldl add (empty, empty) dict


-- LISTS


{-| Get all of the keys in a dictionary, sorted from lowest to highest.

    keys (fromList [(0,"Alice"),(1,"Bob")]) == [0,1]
-}
keys : Dict k v -> List k
keys dict =
  foldr (\key value keyList -> key :: keyList) [] dict


{-| Get all of the values in a dictionary, in the order of their keys.

    values (fromList [(0,"Alice"),(1,"Bob")]) == ["Alice", "Bob"]
-}
values : Dict k v -> List v
values dict =
  foldr (\key value valueList -> value :: valueList) [] dict


{-| Convert a dictionary into an association list of key-value pairs, sorted by keys. -}
toList : Dict k v -> List (k,v)
toList dict =
  foldr (\key value list -> (key,value) :: list) [] dict


{-| Convert an association list into a dictionary. -}
fromList : List (comparable,v) -> Dict comparable v
fromList assocs =
  List.foldl (\(key,value) dict -> insert key value dict) empty assocs
