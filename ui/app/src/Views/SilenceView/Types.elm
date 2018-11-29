module Views.SilenceView.Types exposing (Model, SilenceViewMsg(..), initSilenceView)

import Browser.Navigation exposing (Key)
import Data.GettableAlert exposing (GettableAlert)
import Data.GettableSilence exposing (GettableSilence)
import Utils.Types exposing (ApiData(..))


type SilenceViewMsg
    = FetchSilence String
    | SilenceFetched (ApiData GettableSilence)
    | SetActiveAlert (Maybe String)
    | AlertGroupsPreview (ApiData (List GettableAlert))
    | InitSilenceView String
    | ConfirmDestroySilence GettableSilence Bool
    | Reload String


type alias Model =
    { silence : ApiData GettableSilence
    , alerts : ApiData (List GettableAlert)
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
