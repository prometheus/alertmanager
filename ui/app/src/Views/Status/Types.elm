module Views.Status.Types exposing (StatusModel, StatusMsg(..), initStatusModel)

import Data.AlertmanagerStatus exposing (AlertmanagerStatus)
import Status.Types exposing (StatusResponse)
import Utils.Types exposing (ApiData(..))


type StatusMsg
    = NewStatus (ApiData AlertmanagerStatus)
      -- String carries the api url.
    | InitStatusView String


type alias StatusModel =
    { statusInfo : ApiData AlertmanagerStatus
    }


initStatusModel : StatusModel
initStatusModel =
    { statusInfo = Initial }
