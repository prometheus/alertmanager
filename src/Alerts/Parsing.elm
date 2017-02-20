module Alerts.Parsing exposing (..)

import Alerts.Types exposing (..)
import Utils.Types
import Utils.Parsing
import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)


boolParam : String -> UrlParser.QueryParser (Maybe Bool -> a) a
boolParam name =
    UrlParser.customParam name
        (\x ->
            case x of
                Nothing ->
                    Nothing

                Just value ->
                    -- TODO: Handle Just False case
                    if (String.toLower value) == "false" then
                        Just False
                    else
                        Just True
        )


alertsParser : Parser (Route -> a) a
alertsParser =
    map Receiver (s "alerts" <?> stringParam "receiver" <?> boolParam "silenced" <?> stringParam "filter")
