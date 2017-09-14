module Views.SilenceView.Views exposing (view)

import Alerts.Types exposing (Alert)
import Dialog
import Html exposing (Html, b, button, div, h1, h2, h3, label, p, span, text)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, stateToString)
import Types exposing (Msg(MsgForSilenceList, MsgForSilenceView))
import Utils.Date exposing (dateTimeFormat)
import Utils.List
import Utils.Types exposing (ApiData(Failure, Initial, Loading, Success))
import Utils.Views exposing (error, loading)
import Views.Shared.SilencePreview
import Views.SilenceList.SilenceView exposing (editButton)
import Views.SilenceList.Types exposing (SilenceListMsg(DestroySilence))
import Views.SilenceView.Types exposing (Model, SilenceViewMsg(ConfirmDestroySilence, Reload))


view : Model -> Html Msg
view { silence, alerts, showConfirmationDialog } =
    case silence of
        Success sil ->
            if showConfirmationDialog then
                viewSilence alerts sil True
            else
                viewSilence alerts sil False

        Initial ->
            loading

        Loading ->
            loading

        Failure msg ->
            error msg


viewSilence : ApiData (List Alert) -> Silence -> Bool -> Html Msg
viewSilence alerts silence showPromptDialog =
    div []
        [ h1 []
            [ text "Silence"
            , span
                [ class "ml-3" ]
                [ editButton silence
                , expireButton silence False
                ]
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
        , Dialog.view
            (if showPromptDialog then
                Just (confirmSilenceDeleteView silence True)
             else
                Nothing
            )
        ]


confirmSilenceDeleteView : Silence -> Bool -> Dialog.Config Msg
confirmSilenceDeleteView silence refresh =
    { closeMessage = Just (MsgForSilenceView (Reload silence.id))
    , containerClass = Nothing
    , header = Just (h3 [] [ text "Expire Silence" ])
    , body = Just (text "Are you sure you want to expire this silence?")
    , footer =
        Just
            (button
                [ class "btn btn-success"
                , onClick (MsgForSilenceList (DestroySilence silence refresh))
                ]
                [ text "Confirm" ]
            )
    }


formGroup : String -> Html Msg -> Html Msg
formGroup key content =
    div [ class "form-group row" ]
        [ label [ class "col-2 col-form-label" ] [ b [] [ text key ] ]
        , div [ class "col-10 d-flex align-items-center" ]
            [ content
            ]
        ]


expireButton : Silence -> Bool -> Html Msg
expireButton silence refresh =
    case silence.status.state of
        Silences.Types.Expired ->
            text ""

        Silences.Types.Active ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceView (ConfirmDestroySilence silence refresh))
                ]
                [ text "Expire"
                ]

        Silences.Types.Pending ->
            button
                [ class "btn btn-outline-danger border-0"
                , onClick (MsgForSilenceView (ConfirmDestroySilence silence refresh))
                ]
                [ text "Delete"
                ]
