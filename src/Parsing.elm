module Parsing exposing (..)

-- External Imports

import Alerts.Parsing exposing (alertsParser)
import Navigation
import Types exposing (Route(..))
import UrlParser exposing ((</>), Parser, int, map, oneOf, parseHash, s, string)
import Regex


-- Internal Imports

import Types exposing (Route(..))


-- Parsing


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


silencesParser : Parser a a
silencesParser =
    s "silences"


newSilenceParser : Parser a a
newSilenceParser =
    s "silences" </> s "new"


silenceParser : Parser (Int -> a) a
silenceParser =
    s "silences" </> int


editSilenceParser : Parser (Int -> a) a
editSilenceParser =
    s "silences" </> int </> s "edit"


topLevelParser : Parser a a
topLevelParser =
    s ""


routeParser : Parser (Route -> a) a
routeParser =
    oneOf
        [ map SilencesRoute silencesParser
        , map NewSilenceRoute newSilenceParser
        , map EditSilenceRoute editSilenceParser
        , map SilenceRoute silenceParser
        , map AlertsRoute alertsParser
        , map TopLevel topLevelParser
        ]
