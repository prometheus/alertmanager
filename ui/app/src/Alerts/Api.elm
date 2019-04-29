module Alerts.Api exposing (fetchAlerts, fetchReceivers)

import Data.GettableAlert exposing (GettableAlert)
import Data.Receiver exposing (Receiver)
import Json.Decode
import Regex
import Utils.Api exposing (iso8601Time)
import Utils.Filter exposing (Filter, generateAPIQueryString)
import Utils.Types exposing (ApiData)


fetchReceivers : String -> Cmd (ApiData (List Receiver))
fetchReceivers apiUrl =
    Utils.Api.send
        (Utils.Api.get
            (apiUrl ++ "/receivers")
            (Json.Decode.list Data.Receiver.decoder)
        )


fetchAlerts : String -> Filter -> Cmd (ApiData (List GettableAlert))
fetchAlerts apiUrl filter =
    let
        url =
            String.join "/" [ apiUrl, "alerts" ++ generateAPIQueryString filter ]
    in
    Utils.Api.send (Utils.Api.get url (Json.Decode.list Data.GettableAlert.decoder))
