module Views.Status.Types exposing (StatusModel, StatusMsg(..), initStatusModel)

import Status.Types exposing (StatusResponse)
import Utils.Types exposing (ApiData(..))


type StatusMsg
    = NewStatus (ApiData StatusResponse)
    | InitStatusView


type alias StatusModel =
    { statusInfo : ApiData StatusResponse
    }


initStatusModel : StatusModel
initStatusModel =
    { statusInfo = Initial }
