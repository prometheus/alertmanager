module Parsing exposing (routeParser, urlParser)

import Navigation
import Regex
import Types exposing (Route(..))
import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam, top)
import Views.AlertList.Parsing exposing (alertsParser)
import Views.SilenceForm.Parsing exposing (silenceFormEditParser, silenceFormNewParser)
import Views.SilenceList.Parsing exposing (silenceListParser)
import Views.SilenceView.Parsing exposing (silenceViewParser)
import Views.Status.Parsing exposing (statusParser)


urlParser : Navigation.Location -> Route
urlParser location =
    let
        -- Parse a query string occurring after the hash if it exists, and use
        -- it for routing.
        hashAndQuery =
            Regex.split (Regex.AtMost 1) (Regex.regex "\\?") location.hash

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
            NotFoundRoute


routeParser : Parser (Route -> a) a
routeParser =
    oneOf
        [ map SilenceListRoute silenceListParser
        , map StatusRoute statusParser
        , map SilenceFormNewRoute silenceFormNewParser
        , map SilenceViewRoute silenceViewParser
        , map SilenceFormEditRoute silenceFormEditParser
        , map AlertsRoute alertsParser
        , map TopLevelRoute top
        ]
