module Parsing exposing (..)

import Alerts.Parsing exposing (alertsParser)
import Silences.Parsing exposing (silencesParser)
import Status.Parsing exposing (statusParser)
import Navigation
import Types exposing (Route(..))
import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)
import Regex
import Types exposing (Route(..))


urlParser : Navigation.Location -> Route
urlParser location =
    let
        -- Parse a query string occurring after the hash if it exists, and use
        -- it for routing.
        hashAndQuery =
            Regex.split (Regex.AtMost 1) (Regex.regex "\\?") (location.hash)

        hash =
            case List.head hashAndQuery of
                Just hash ->
                    hash

                Nothing ->
                    ""

        query =
            if List.length hashAndQuery == 2 then
                case List.head <| List.reverse hashAndQuery of
                    Just query ->
                        "?" ++ query

                    Nothing ->
                        ""
            else
                ""
    in
        case parseHash routeParser { location | search = query, hash = hash } of
            Just route ->
                route

            Nothing ->
                NotFound


topLevelParser : Parser a a
topLevelParser =
    s ""


routeParser : Parser (Route -> a) a
routeParser =
    oneOf
        [ map SilencesRoute silencesParser
        , map StatusRoute statusParser
        , map AlertsRoute alertsParser
        , map TopLevel topLevelParser
        ]
