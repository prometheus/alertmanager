module Parsing exposing (..)

-- External Imports

import Navigation
import UrlParser exposing (Parser, (</>), map, int, oneOf, s, string)
import String


-- Internal Imports

import Types exposing (Route(..))


-- Parsing


urlParser : Navigation.Location -> Route
urlParser location =
    let
        one =
            Debug.log "hash" location.hash
    in
        case UrlParser.parseHash routeParser location of
            Just route ->
                route

            Nothing ->
                NotFound


silencesParser : Parser a a
silencesParser =
    UrlParser.s "silences"


silenceParser : Parser (String -> a) a
silenceParser =
    UrlParser.s "silence" </> UrlParser.string


alertsParser : Parser a a
alertsParser =
    UrlParser.s "alerts"


alertParser : Parser (String -> a) a
alertParser =
    UrlParser.s "alert" </> UrlParser.string


topLevelParser : Parser a a
topLevelParser =
    UrlParser.s ""


routeParser : Parser (Route -> a) a
routeParser =
    UrlParser.oneOf
        [ map SilencesRoute silencesParser
        , map SilenceRoute silenceParser
        , map AlertsRoute alertsParser
        , map AlertRoute alertParser
        , map TopLevel topLevelParser
        ]
