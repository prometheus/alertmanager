module Alerts.Api exposing (fetchAlerts, fetchReceivers)

import Data.GettableAlerts exposing (GettableAlerts)
import Data.Receiver exposing (Receiver)
import Json.Decode
import Regex
import Utils.Api exposing (iso8601Time)
import Utils.Filter exposing (Filter, generateQueryString)
import Utils.Types exposing (ApiData)


fetchReceivers : String -> Cmd (ApiData (List Receiver))
fetchReceivers apiUrl =
    Utils.Api.send
        (Utils.Api.get
            (apiUrl ++ "/receivers")
            (Json.Decode.list Data.Receiver.decoder)
        )


fetchAlerts : String -> Filter -> Cmd (ApiData GettableAlerts)
fetchAlerts apiUrl filter =
    let
        url =
            String.join "/" [ apiUrl, "alerts" ++ generateQueryString filter ]
    in
    Utils.Api.send (Utils.Api.get url Data.GettableAlerts.decoder)
