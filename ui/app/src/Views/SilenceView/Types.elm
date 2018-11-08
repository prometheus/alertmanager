module Views.SilenceView.Types exposing (Model, SilenceViewMsg(..), initSilenceView)

import Alerts.Types exposing (Alert)
import Browser.Navigation exposing (Key)
import Data.Silence exposing (Silence)
import Data.Silences exposing (Silences)
import Utils.Types exposing (ApiData(..))


type SilenceViewMsg
    = FetchSilence String
    | SilenceFetched (ApiData Silence)
    | SetActiveAlert (Maybe String)
    | AlertGroupsPreview (ApiData (List Alert))
    | InitSilenceView String
    | ConfirmDestroySilence Silence Bool
    | Reload String


type alias Model =
    { silence : ApiData Silence
    , alerts : ApiData (List Alert)
    , activeAlertId : Maybe String
    , showConfirmationDialog : Bool
    , key : Key
    }


initSilenceView : Key -> Model
initSilenceView key =
    { silence = Initial
    , alerts = Initial
    , activeAlertId = Nothing
    , showConfirmationDialog = False
    , key = key
    }
