module Views.SilenceView.Types exposing (Model, SilenceViewMsg(..), initSilenceView)

import Alerts.Types exposing (Alert)
import Browser.Navigation exposing (Key)
import Silences.Types exposing (Silence, SilenceId)
import Utils.Types exposing (ApiData(..))


type SilenceViewMsg
    = FetchSilence String
    | SilenceFetched (ApiData Silence)
    | AlertGroupsPreview (ApiData (List Alert))
    | InitSilenceView SilenceId
    | ConfirmDestroySilence Silence Bool
    | Reload String


type alias Model =
    { silence : ApiData Silence
    , alerts : ApiData (List Alert)
    , showConfirmationDialog : Bool
    , key : Key
    }


initSilenceView : Key -> Model
initSilenceView key =
    { silence = Initial
    , alerts = Initial
    , showConfirmationDialog = False
    , key = key
    }
