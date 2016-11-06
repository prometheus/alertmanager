module Parsing exposing (..)

-- External Imports

import Navigation
import UrlParser exposing (Parser, (</>), format, int, oneOf, s, string)
import String


-- Internal Imports

import Types exposing (Route(..))


-- Parsing


parse : Navigation.Location -> Route
parse { pathname, hash } =
    let
        one =
            Debug.log "parse: hash" hash

        path =
            if String.startsWith "#/" hash then
                String.dropLeft 2 hash
            else
                hash
    in
        case UrlParser.parse identity routeParser path of
            Err err ->
                NotFound

            Ok route ->
                route


urlParser : Navigation.Parser Route
urlParser =
    Navigation.makeParser parse


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
        [ format SilencesRoute silencesParser
        , format SilenceRoute silenceParser
        , format AlertsRoute alertsParser
        , format AlertRoute alertParser
        , format TopLevel topLevelParser
        ]
