module Utils.Filter exposing (..)

import Utils.Types exposing (Filter)
import Http exposing (encodeUri)


generateQueryParam : String -> Maybe String -> Maybe String
generateQueryParam name =
    Maybe.map (encodeUri >> (++) (name ++ "="))


generateQueryString : Filter -> String
generateQueryString { receiver, showSilenced, text } =
    [ ( "receiver", receiver )
    , ( "silenced", Maybe.map (toString >> String.toLower) showSilenced )
    , ( "filter", text )
    ]
        |> List.filterMap (uncurry generateQueryParam)
        |> String.join "&"
        |> (++) "?"
