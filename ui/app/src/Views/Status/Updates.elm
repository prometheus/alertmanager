module Views.Status.Updates exposing (update)

import Status.Api exposing (getStatus)
import Types exposing (Model, Msg(..))
import Views.Status.Types exposing (StatusMsg(..))


update : StatusMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        NewStatus apiResponse ->
            ( { model | status = { statusInfo = apiResponse } }, Cmd.none )

        InitStatusView apiUrl ->
            ( model, getStatus apiUrl (NewStatus >> MsgForStatus) )
