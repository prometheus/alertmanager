module Views.SilenceList.Types exposing (Model, SilenceListMsg(..), SilenceTab, initSilenceList)

import Browser.Navigation exposing (Key)
import Data.GettableSilence exposing (GettableSilence)
import Data.SilenceStatus exposing (State(..))
import Time exposing (Posix)
import Utils.Types exposing (ApiData(..))
import Views.FilterBar.Types as FilterBar


type SilenceListMsg
    = ConfirmDestroySilence GettableSilence Bool
    | DestroySilence GettableSilence Bool
    | SilencesFetch (ApiData (List GettableSilence))
    | FetchSilences
    | MsgForFilterBar FilterBar.Msg
    | SetTab State
    | SetTimeToSilenceList


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
    , timeNow : Posix
    , key : Key
    }


initSilenceList : Key -> Posix -> Model
initSilenceList key now =
    { silences = Initial
    , filterBar = FilterBar.initFilterBar key
    , tab = Active
    , showConfirmationDialog = Nothing
    , timeNow = now
    , key = key
    }
