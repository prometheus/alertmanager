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


newSilenceParser : Parser a a
newSilenceParser =
    UrlParser.s "silences" </> UrlParser.s "new"


silenceParser : Parser (Int -> a) a
silenceParser =
    UrlParser.s "silences" </> UrlParser.int


editSilenceParser : Parser (Int -> a) a
editSilenceParser =
    UrlParser.s "silences" </> UrlParser.int </> UrlParser.s "edit"


alertsParser : Parser a a
alertsParser =
    UrlParser.s "alerts"


topLevelParser : Parser a a
topLevelParser =
    UrlParser.s ""


routeParser : Parser (Route -> a) a
routeParser =
    UrlParser.oneOf
        [ map SilencesRoute silencesParser
        , map NewSilenceRoute newSilenceParser
        , map EditSilenceRoute editSilenceParser
        , map SilenceRoute silenceParser
        , map AlertGroupsRoute alertsParser
        , map TopLevel topLevelParser
        ]
