module Alerts.Parsing exposing (..)

import Alerts.Types exposing (..)
import UrlParser exposing ((</>), Parser, int, map, oneOf, parseHash, s, string)


alertsParser : Parser (Route -> a) a
alertsParser =
    map Route (s "alerts")
