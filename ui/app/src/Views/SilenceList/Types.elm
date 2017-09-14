module Views.SilenceList.Types exposing (Model, SilenceListMsg(..), initSilenceList)

import Silences.Types exposing (Silence, State(Active))
import Utils.Types exposing (ApiData(Initial))
import Views.FilterBar.Types as FilterBar


type SilenceListMsg
    = ConfirmDestroySilence Silence Bool
    | DestroySilence Silence Bool
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
    | MsgForFilterBar FilterBar.Msg
    | SetTab State


type alias Model =
    { silences : ApiData (List Silence)
    , filterBar : FilterBar.Model
    , tab : State
    , showConfirmationDialog : Bool
    }


initSilenceList : Model
initSilenceList =
    { silences = Initial
    , filterBar = FilterBar.initFilterBar
    , tab = Active
    , showConfirmationDialog = False
    }
