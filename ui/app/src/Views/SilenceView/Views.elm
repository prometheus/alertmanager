module Views.SilenceView.Views exposing (view)

import Silences.Types exposing (Silence, stateToString, State(Expired, Active, Pending))
import Alerts.Types exposing (Alert)
import Html exposing (Html, div, h2, p, text, label, b, h1, a, button, span)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Types exposing (Msg(Noop, MsgForSilenceView, MsgForSilenceForm))
import Views.SilenceForm.Types exposing (SilenceFormMsg(NewSilenceFromMatchers))
import Utils.Types exposing (ApiData(Initial, Success, Loading, Failure))
import Utils.Views exposing (loading, error)
import Views.Shared.SilencePreview
import Views.SilenceView.Types exposing (Model, SilenceViewMsg(DestroySilence))
import Utils.Date exposing (dateTimeFormat)
import Utils.List


view : Model -> Html Msg
view { silence, alerts } =
    case silence of
        Success sil ->
            viewSilence alerts sil

        Initial ->
            loading

        Loading ->
            loading

        Failure msg ->
            error msg


viewSilence : ApiData (List Alert) -> Silence -> Html Msg
viewSilence alerts silence =
    div []
        [ h1 [ class "d-inline-block" ] [ text "Silence" ]
        , span []
            [ editButton silence
            , deleteButton silence
            ]
        , formGroup "ID" <| text silence.id
        , formGroup "Starts at" <| text <| dateTimeFormat silence.startsAt
        , formGroup "Ends at" <| text <| dateTimeFormat silence.endsAt
        , formGroup "Updated at" <| text <| dateTimeFormat silence.updatedAt
        , formGroup "Created by" <| text silence.createdBy
        , formGroup "Comment" <| text silence.comment
        , formGroup "State" <| text <| stateToString silence.status.state
        , formGroup "Matchers" <|
            div [] <|
                List.map (Utils.List.mstring >> Utils.Views.labelButton Nothing) silence.matchers
        , formGroup "Affected alerts" <| Views.Shared.SilencePreview.view alerts
        ]


formGroup : String -> Html Msg -> Html Msg
formGroup key content =
    div [ class "form-group row" ]
        [ label [ class "col-2 col-form-label" ] [ b [] [ text key ] ]
        , div [ class "col-10 d-flex align-items-center" ]
            [ content
            ]
        ]


editButton : Silence -> Html Msg
editButton silence =
    case silence.status.state of
        -- If the silence is expired, do not edit it, but instead create a new
        -- one with the old matchers
        Expired ->
            a
                [ class "btn btn-outline-info border-0"
                , href ("#/silences/new?keep=1")
                , onClick (NewSilenceFromMatchers silence.matchers |> MsgForSilenceForm)
                ]
                [ text "Recreate"
                ]

        _ ->
            let
                editUrl =
                    String.join "/" [ "#/silences", silence.id, "edit" ]
            in
                a [ class "btn btn-outline-info border-0", href editUrl ]
                    [ text "Edit"
                    ]


deleteButton : Silence -> Html Msg
deleteButton silence =
    case silence.status.state of
        Expired ->
            text ""

        Active ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceView (DestroySilence silence))
                ]
                [ text "Expire"
                ]

        Pending ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceView (DestroySilence silence))
                ]
                [ text "Delete"
                ]
