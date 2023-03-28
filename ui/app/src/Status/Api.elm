module Status.Api exposing (clusterStatusToString, getStatus)

import Data.AlertmanagerStatus exposing (AlertmanagerStatus)
import Data.ClusterStatus exposing (Status(..))
import Utils.Api exposing (get, send)
import Utils.Types exposing (ApiData)


getStatus : String -> (ApiData AlertmanagerStatus -> msg) -> Cmd msg
getStatus apiUrl msg =
    let
        url =
            String.join "/" [ apiUrl, "status" ]

        request =
            get url Data.AlertmanagerStatus.decoder
    in
    Cmd.map msg <| send request


clusterStatusToString : Status -> String
clusterStatusToString status =
    case status of
        Ready ->
            "ready"

        Settling ->
            "settling"

        Disabled ->
            "disabled"
