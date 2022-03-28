module Views.SilenceList.Types exposing (Model, SilenceListMsg(..), SilenceTab, initSilenceList)

import Browser.Navigation exposing (Key)
import Data.GettableSilence exposing (GettableSilence)
import Data.SilenceStatus exposing (State(..))
import Utils.Types exposing (ApiData(..))
import Views.FilterBar.Types as FilterBar


type SilenceListMsg
    = ConfirmDestroySilence GettableSilence
    | DestroySilence GettableSilence Bool
    | SilencesFetch (ApiData (List GettableSilence))
    | FetchSilences
    | MsgForFilterBar FilterBar.Msg
    | SetTab State


type alias SilenceTab =
    { silences : List GettableSilence
    , tab : State
    , count : Int
    }


type alias Model =
    { silences : ApiData (List SilenceTab)
    , filterBar : FilterBar.Model
    , tab : State
    , showConfirmationDialog : Maybe String
    , key : Key
    }


initSilenceList : Key -> Model
initSilenceList key =
    { silences = Initial
    , filterBar = FilterBar.initFilterBar []
    , tab = Active
    , showConfirmationDialog = Nothing
    , key = key
    }
