module Views.Status.Types exposing (StatusMsg(..), StatusModel, initStatusModel)

import Status.Types exposing (StatusResponse)
import Utils.Types exposing (ApiData(Initial))


type StatusMsg
    = NewStatus (ApiData StatusResponse)
    | InitStatusView


type alias StatusModel =
    { statusInfo : ApiData StatusResponse
    }


initStatusModel : StatusModel
initStatusModel =
    { statusInfo = Initial }
