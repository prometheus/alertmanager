module Utils.List exposing (groupBy, lastElem, mjoin, mstring, nextElem, replaceIf, replaceIndex, zip)

import Data.Matcher exposing (Matcher)
import Dict exposing (Dict)


nextElem : a -> List a -> Maybe a
nextElem el list =
    case list of
        curr :: rest ->
            if curr == el then
                List.head rest

            else
                nextElem el rest

        [] ->
            Nothing


lastElem : List a -> Maybe a
lastElem =
    List.foldl (Just >> always) Nothing


replaceIf : (a -> Bool) -> a -> List a -> List a
replaceIf predicate replacement list =
    List.map
        (\item ->
            if predicate item then
                replacement

            else
                item
        )
        list


replaceIndex : Int -> (a -> a) -> List a -> List a
replaceIndex index replacement list =
    List.indexedMap
        (\currentIndex item ->
            if index == currentIndex then
                replacement item

            else
                item
        )
        list


mjoin : List Matcher -> String
mjoin m =
    String.join "," (List.map mstring m)


mstring : Matcher -> String
mstring m =
    let
        sep =
            if m.isRegex then
                "=~"

            else
                "="
    in
    String.join sep [ m.name, m.value ]


{-| Takes a key-fn and a list.
Creates a `Dict` which maps the key to a list of matching elements.
mary = {id=1, name="Mary"}
jack = {id=2, name="Jack"}
jill = {id=1, name="Jill"}
groupBy .id [mary, jack, jill] == Dict.fromList [(1, [mary, jill]), (2, [jack])]

Copied from <https://github.com/elm-community/dict-extra/blob/2.0.0/src/Dict/Extra.elm>

-}
groupBy : (a -> comparable) -> List a -> Dict comparable (List a)
groupBy keyfn list =
    List.foldr
        (\x acc ->
            Dict.update (keyfn x) (Maybe.map ((::) x) >> Maybe.withDefault [ x ] >> Just) acc
        )
        Dict.empty
        list


zip : List a -> List b -> List ( a, b )
zip a b =
    List.map2 (\a1 b1 -> ( a1, b1 )) a b
