module Utils.Filter exposing (..)

import QueryString exposing (QueryString, empty, add, render)
import Utils.Types exposing (Filter)


generateQueryString : Filter -> String
generateQueryString filter =
    empty
        |> addMaybe "receiver" filter.receiver identity
        |> addMaybe "silenced" filter.showSilenced (toString >> String.toLower)
        |> addMaybe "filter" filter.text identity
        |> render


addMaybe : String -> Maybe a -> (a -> String) -> QueryString -> QueryString
addMaybe key maybeValue stringFn qs =
    case maybeValue of
        Just value ->
            add key (stringFn value) qs

        Nothing ->
            qs
