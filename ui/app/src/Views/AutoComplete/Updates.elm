module Views.AutoComplete.Updates exposing (update)

import Views.AutoComplete.Types exposing (Model, Msg(..))


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    ( model, Cmd.none )
