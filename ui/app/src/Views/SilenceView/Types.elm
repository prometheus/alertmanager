module Views.SilenceView.Types exposing (SilenceViewMsg(..), Model, initSilenceView)

import Silences.Types exposing (Silence, SilenceId)
import Alerts.Types exposing (Alert)
import Utils.Types exposing (ApiData(Initial))


type SilenceViewMsg
    = FetchSilence String
    | SilenceFetched (ApiData Silence)
    | AlertGroupsPreview (ApiData (List Alert))
    | InitSilenceView SilenceId


type alias Model =
    { silence : ApiData Silence
    , alerts : ApiData (List Alert)
    }


initSilenceView : Model
initSilenceView =
    { silence = Initial
    , alerts = Initial
    }
