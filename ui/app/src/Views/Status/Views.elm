module Views.Status.Views exposing (view)

import Html exposing (Html, text, button, div, li, ul, b)
import Status.Types exposing (StatusResponse)
import Types exposing (Msg(MsgForStatus), Model)
import Utils.Types exposing (ApiResponse(Failure, Success, Loading), ApiData)


view : Model -> Html Types.Msg
view model =
    ul []
        [ li []
            [ b [] [ text "Status: " ], text (getStatus model.status.statusInfo) ]
        , li []
            [ b [] [ text "Uptime: " ], text (getUptime model.status.statusInfo) ]
        ]


getStatus : ApiData StatusResponse -> String
getStatus apiResponse =
    case apiResponse of
        Failure e ->
            "Error occured when requesting status information: " ++ (toString e)

        Loading ->
            "Loading"

        Success a ->
            a.status


getUptime : ApiData StatusResponse -> String
getUptime apiResponse =
    case apiResponse of
        Failure e ->
            "Error occured when requesting status information: " ++ (toString e)

        Loading ->
            "Loading"

        Success a ->
            a.uptime
