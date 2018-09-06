module Views.SilenceList.Types exposing (Model, SilenceListMsg(..), SilenceTab, initSilenceList)

import Browser.Navigation exposing (Key)
import Silences.Types exposing (Silence, SilenceId, State(..))
import Utils.Types exposing (ApiData(..))
import Views.FilterBar.Types as FilterBar


type SilenceListMsg
    = ConfirmDestroySilence Silence Bool
    | DestroySilence Silence Bool
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
    | MsgForFilterBar FilterBar.Msg
    | SetTab State


type alias SilenceTab =
    { silences : List Silence
    , tab : State
    , count : Int
    }


type alias Model =
    { silences : ApiData (List SilenceTab)
    , filterBar : FilterBar.Model
    , tab : State
    , showConfirmationDialog : Maybe SilenceId
    , key : Key
    }


initSilenceList : Key -> Model
initSilenceList key =
    { silences = Initial
    , filterBar = FilterBar.initFilterBar key
    , tab = Active
    , showConfirmationDialog = Nothing
    , key = key
    }
