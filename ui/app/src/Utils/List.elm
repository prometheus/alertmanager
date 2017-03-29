module Utils.List exposing (..)

import Utils.Types exposing (Matchers, Matcher)


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


mjoin : Matchers -> String
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
        String.join sep [ m.name, toString m.value ]
