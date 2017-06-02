module Views.Status.Updates exposing (update)

import Types exposing (Msg(MsgForStatus), Model)
import Views.Status.Types exposing (StatusMsg(..))
import Status.Api exposing (getStatus)


update : StatusMsg -> Model -> String -> ( Model, Cmd Msg )
update msg model basePath =
    case msg of
        NewStatus apiResponse ->
            ( { model | status = { statusInfo = apiResponse } }, Cmd.none )

        InitStatusView ->
            ( model, getStatus basePath (NewStatus >> MsgForStatus) )
