module Views.SilenceView.Types exposing (Model, SilenceViewMsg(..), initSilenceView)

import Browser.Navigation exposing (Key)
import Data.Alerts exposing (Alerts)
import Data.Silence exposing (Silence)
import Data.Silences exposing (Silences)
import Utils.Types exposing (ApiData(..))


type SilenceViewMsg
    = FetchSilence String
    | SilenceFetched (ApiData Silence)
    | AlertGroupsPreview (ApiData Alerts)
    | InitSilenceView String
    | ConfirmDestroySilence Silence Bool
    | Reload String


type alias Model =
    { silence : ApiData Silence
    , alerts : ApiData Alerts
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
