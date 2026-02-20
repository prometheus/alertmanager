module Utils.List exposing (groupBy, lastElem, mstring, nextElem, zip)

import Data.Matcher exposing (Matcher)
import Dict exposing (Dict)
import Json.Encode as Encode


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


mstring : Matcher -> String
mstring m =
    let
        isEqual =
            case m.isEqual of
                Nothing ->
                    True

                Just value ->
                    value

        sep =
            if not m.isRegex && isEqual then
                "="

            else if not m.isRegex && not isEqual then
                "!="

            else if m.isRegex && isEqual then
                "=~"

            else
                "!~"
    in
    String.join sep [ m.name, Encode.encode 0 (Encode.string m.value) ]


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
