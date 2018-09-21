module Alerts.Api exposing (fetchAlerts, fetchReceivers)

import Alerts.Types exposing (Alert, Receiver)
import Data.Alerts exposing (Alerts)
import Json.Decode as Json exposing (..)
import Regex
import Utils.Api exposing (iso8601Time)
import Utils.Filter exposing (Filter, generateQueryString)
import Utils.Types exposing (ApiData)


escapeRegExp : String -> String
escapeRegExp text =
    let
        reg =
            Regex.fromString "/[-[\\]{}()*+?.,\\\\^$|#\\s]/g" |> Maybe.withDefault Regex.never
    in
    Regex.replace reg (.match >> (++) "\\") text


fetchReceivers : String -> Cmd (ApiData (List Receiver))
fetchReceivers apiUrl =
    Utils.Api.send
        (Utils.Api.get
            (apiUrl ++ "/receivers")
            (field "data" (list (Json.map (\receiver -> Receiver receiver (escapeRegExp receiver)) string)))
        )


fetchAlerts : String -> Filter -> Cmd (ApiData Alerts)
fetchAlerts apiUrl filter =
    let
        url =
            String.join "/" [ apiUrl, "alerts" ++ generateQueryString filter ]
    in
    Utils.Api.send (Utils.Api.get url Data.Alerts.decoder)
