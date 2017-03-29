module Views.Silence.Views exposing (view)

import Silences.Types exposing (Silence)
import Html exposing (Html, div, h2, p, text, label)
import Html.Attributes exposing (class)
import Time exposing (Time)
import Types exposing (Model, Msg)
import Utils.Types exposing (ApiResponse(Success, Loading, Failure))
import Utils.Views exposing (loading, error)
import Views.Shared.SilencePreview
import Views.Shared.SilenceBase


view : Model -> Html Msg
view model =
    case model.silence of
        Success sil ->
            silence sil model.currentTime

        Loading ->
            loading

        Failure msg ->
            error msg


silence : Silence -> Time -> Html Msg
silence silence currentTime =
    div []
        [ Views.Shared.SilenceBase.view silence
        , silenceExtra silence currentTime
        , h2 [ class "h6 dark-red" ] [ text "Affected alerts" ]
        , Views.Shared.SilencePreview.view silence
        ]


silenceExtra : Silence -> Time -> Html msg
silenceExtra silence currentTime =
    div [ class "f6" ]
        [ div [ class "mb1" ]
            [ p []
                [ text "Status: "
                , Utils.Views.button "ph3 pv2" (status silence currentTime)
                ]
            , div []
                [ label [ class "f6 dib mb2 mr2 w-40" ] [ text "Created by" ]
                , p [] [ text silence.createdBy ]
                ]
            , div []
                [ label [ class "f6 dib mb2 mr2 w-40" ] [ text "Comment" ]
                , p [] [ text silence.comment ]
                ]
            ]
        ]


status : Silence -> Time -> String
status { endsAt, startsAt } currentTime =
    if endsAt <= currentTime then
        "expired"
    else if startsAt > currentTime then
        "pending"
    else
        "active"
