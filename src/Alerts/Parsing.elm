module Alerts.Parsing exposing (..)

import Alerts.Types exposing (..)
import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)


alertsParser : Parser (Route -> a) a
alertsParser =
    oneOf
        [ map AllReceivers (s "alerts")
        , map Receiver (s "alerts" </> string)
        ]
