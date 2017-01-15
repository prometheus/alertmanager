module Parsing exposing (..)

-- External Imports

import Navigation
import UrlParser exposing (Parser, (</>), map, int, oneOf, s, string, parseHash)
import String


-- Internal Imports

import Types exposing (Route(..))


-- Parsing


urlParser : Navigation.Location -> Route
urlParser location =
    case parseHash routeParser location of
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


alertsParser : Parser a a
alertsParser =
    s "alerts"


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
        , map AlertGroupsRoute alertsParser
        , map TopLevel topLevelParser
        ]
