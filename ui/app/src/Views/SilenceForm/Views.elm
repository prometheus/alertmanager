module Views.SilenceForm.Views exposing (view)

import Html exposing (Html, a, div, fieldset, label, legend, span, text, h1, strong, button, input, textarea)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, SilenceId)
import Alerts.Types exposing (Alert)
import Views.Shared.SilencePreview
import Views.SilenceForm.Types exposing (Model, SilenceFormMsg(..), MatcherForm)
import Utils.Types exposing (ApiData)
import Utils.Views exposing (checkbox, error, iconButtonMsg, validatedField)
import Utils.FormValidation exposing (ValidationState(..), ValidatedField)
import Views.SilenceForm.Types exposing (Model, SilenceFormMsg(..), SilenceFormFieldMsg(..), SilenceForm)


view : Maybe SilenceId -> Model -> Html SilenceFormMsg
view maybeId { form, alerts } =
    let
        ( title, resetClick ) =
            case maybeId of
                Just silenceId ->
                    ( "Edit Silence", FetchSilence silenceId )

                Nothing ->
                    ( "New Silence", NewSilenceFromMatchers [] )
    in
        div []
            [ div []
                [ h1 [] [ text title ]
                , timeInput form.startsAt form.endsAt form.duration
                , matcherInput form.matchers
                , validatedField input
                    "Creator"
                    inputSectionPadding
                    (UpdateCreatedBy >> UpdateField)
                    (ValidateCreatedBy |> UpdateField)
                    form.createdBy
                , validatedField textarea
                    "Comment"
                    inputSectionPadding
                    (UpdateComment >> UpdateField)
                    (ValidateComment |> UpdateField)
                    form.comment
                , preview alerts
                , silenceActionButtons maybeId form resetClick
                ]
            ]


inputSectionPadding : String
inputSectionPadding =
    "mt-5"


timeInput : ValidatedField -> ValidatedField -> ValidatedField -> Html SilenceFormMsg
timeInput startsAt endsAt duration =
    div [ class <| "row " ++ inputSectionPadding ]
        [ validatedField input
            "Start"
            "col-5"
            (UpdateStartsAt >> UpdateField)
            (ValidateStartsAt |> UpdateField)
            startsAt
        , validatedField input
            "Duration"
            "col-2"
            (UpdateDuration >> UpdateField)
            (ValidateDuration |> UpdateField)
            duration
        , validatedField input
            "End"
            "col-5"
            (UpdateEndsAt >> UpdateField)
            (ValidateEndsAt |> UpdateField)
            endsAt
        ]


matcherInput : List MatcherForm -> Html SilenceFormMsg
matcherInput matchers =
    div [ class inputSectionPadding ]
        [ div []
            [ label []
                [ strong [] [ text "Matchers " ]
                , span [ class "" ] [ text "Alerts affected by this silence." ]
                ]
            , div [ class "row" ]
                [ label [ class "col-5" ] [ text "Name" ]
                , label [ class "col-5" ] [ text "Value" ]
                ]
            ]
        , div [] (List.indexedMap matcherForm matchers)
        , iconButtonMsg "btn btn-secondary" "fa-plus" (AddMatcher |> UpdateField)
        ]


silenceActionButtons : Maybe String -> SilenceForm -> SilenceFormMsg -> Html SilenceFormMsg
silenceActionButtons maybeId form resetClick =
    div [ class inputSectionPadding ]
        [ createSilenceBtn maybeId form
        , button
            [ class "ml-2 btn btn-danger", onClick resetClick ]
            [ text "Reset" ]
        ]


createSilenceBtn : Maybe String -> SilenceForm -> Html SilenceFormMsg
createSilenceBtn maybeId form =
    let
        btnTxt =
            case maybeId of
                Just _ ->
                    "Update"

                Nothing ->
                    "Create"
    in
        button
            [ class "btn btn-primary"
            , onClick CreateSilence
            ]
            [ text btnTxt ]


preview : ApiData (List Alert) -> Html SilenceFormMsg
preview alerts =
    div [ class inputSectionPadding ]
        [ button [ class "btn btn-outline-success", onClick PreviewSilence ] [ text "Load affected Alerts" ]
        , Views.Shared.SilencePreview.view alerts
        ]


matcherForm : Int -> MatcherForm -> Html SilenceFormMsg
matcherForm index { name, value, isRegex } =
    div [ class "row" ]
        [ div [ class "col-5" ] [ validatedField input "" "" (UpdateMatcherName index) (ValidateMatcherName index) name ]
        , div [ class "col-5" ] [ validatedField input "" "" (UpdateMatcherValue index) (ValidateMatcherValue index) value ]
        , div [ class "col-2 d-flex align-items-center" ]
            [ checkbox "Regex" isRegex (UpdateMatcherRegex index)
            , iconButtonMsg "btn btn-secondary ml-auto" "fa-trash-o" (DeleteMatcher index)
            ]
        ]
        |> Html.map UpdateField
