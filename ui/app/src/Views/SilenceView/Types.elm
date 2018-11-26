module Views.SilenceView.Types exposing (Model, SilenceViewMsg(..), initSilenceView)

import Browser.Navigation exposing (Key)
import Data.GettableAlerts exposing (GettableAlerts)
import Data.GettableSilence exposing (GettableSilence)
import Data.GettableSilences exposing (GettableSilences)
import Utils.Types exposing (ApiData(..))


type SilenceViewMsg
    = FetchSilence String
    | SilenceFetched (ApiData GettableSilence)
    | SetActiveAlert (Maybe String)
    | AlertGroupsPreview (ApiData GettableAlerts)
    | InitSilenceView String
    | ConfirmDestroySilence GettableSilence Bool
    | Reload String


type alias Model =
    { silence : ApiData GettableSilence
    , alerts : ApiData GettableAlerts
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
