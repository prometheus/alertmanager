module Views.AlertList.Parsing exposing (alertsParser)

import Views.AlertList.Types exposing (Route(Receiver))
import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)


boolParam : String -> UrlParser.QueryParser (Maybe Bool -> a) a
boolParam name =
    UrlParser.customParam name
        (\x ->
            case x of
                Nothing ->
                    Nothing

                Just value ->
                    if (String.toLower value) == "false" then
                        Just False
                    else
                        Just True
        )


alertsParser : Parser (Route -> a) a
alertsParser =
    map Receiver (s "alerts" <?> stringParam "receiver" <?> boolParam "silenced" <?> stringParam "filter")
