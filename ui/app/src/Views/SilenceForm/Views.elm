module Views.SilenceForm.Views exposing (view)

import Data.GettableAlert exposing (GettableAlert)
import Html exposing (Html, a, button, div, fieldset, h1, input, label, legend, span, strong, text, textarea)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Utils.Filter
import Utils.FormValidation exposing (ValidatedField, ValidationState(..))
import Utils.Types exposing (ApiData)
import Utils.Views exposing (checkbox, iconButtonMsg, loading, validatedField)
import Views.Shared.SilencePreview
import Views.Shared.Types exposing (Msg)
import Views.SilenceForm.Types exposing (MatcherForm, Model, SilenceForm, SilenceFormFieldMsg(..), SilenceFormMsg(..))


view : Maybe String -> List Utils.Filter.Matcher -> String -> Model -> Html SilenceFormMsg
view maybeId matchers defaultCreator { form, silenceId, alerts, activeAlertId } =
    let
        ( title, resetClick ) =
            case maybeId of
                Just silenceId_ ->
                    ( "Edit Silence", FetchSilence silenceId_ )

                Nothing ->
                    ( "New Silence", NewSilenceFromMatchers defaultCreator matchers )
    in
    div []
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
        , div [ class inputSectionPadding ]
            [ informationBlock activeAlertId silenceId alerts
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
            (ValidateTime |> UpdateField)
            startsAt
        , validatedField input
            "Duration"
            "col-2"
            (UpdateDuration >> UpdateField)
            (ValidateTime |> UpdateField)
            duration
        , validatedField input
            "End"
            "col-5"
            (UpdateEndsAt >> UpdateField)
            (ValidateTime |> UpdateField)
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
        , div [] (List.indexedMap (matcherForm (List.length matchers > 1)) matchers)
        , iconButtonMsg "btn btn-secondary" "fa-plus" (AddMatcher |> UpdateField)
        ]


informationBlock : Maybe String -> ApiData String -> ApiData (List GettableAlert) -> Html SilenceFormMsg
informationBlock activeAlertId silence alerts =
    case silence of
        Utils.Types.Success _ ->
            text ""

        Utils.Types.Initial ->
            Views.Shared.SilencePreview.view activeAlertId alerts
                |> Html.map SetActiveAlert

        Utils.Types.Failure error ->
            Utils.Views.error error

        Utils.Types.Loading ->
            loading


silenceActionButtons : Maybe String -> SilenceForm -> SilenceFormMsg -> Html SilenceFormMsg
silenceActionButtons maybeId form resetClick =
    div [ class ("mb-4 " ++ inputSectionPadding) ]
        [ previewSilenceBtn
        , createSilenceBtn maybeId
        , button
            [ class "ml-2 btn btn-danger", onClick resetClick ]
            [ text "Reset" ]
        ]


createSilenceBtn : Maybe String -> Html SilenceFormMsg
createSilenceBtn maybeId =
    let
        btnTxt =
            case maybeId of
                Just _ ->
                    "Update"

                Nothing ->
                    "Create"
    in
    button
        [ class "ml-2 btn btn-primary"
        , onClick CreateSilence
        ]
        [ text btnTxt ]


previewSilenceBtn : Html SilenceFormMsg
previewSilenceBtn =
    button
        [ class "btn btn-outline-success"
        , onClick PreviewSilence
        ]
        [ text "Preview Alerts" ]


matcherForm : Bool -> Int -> MatcherForm -> Html SilenceFormMsg
matcherForm showDeleteButton index { name, value, isRegex } =
    div [ class "row" ]
        [ div [ class "col-5" ] [ validatedField input "" "" (UpdateMatcherName index) (ValidateMatcherName index) name ]
        , div [ class "col-5" ] [ validatedField input "" "" (UpdateMatcherValue index) (ValidateMatcherValue index) value ]
        , div [ class "col-2 d-flex align-items-center" ]
            [ checkbox "Regex" isRegex (UpdateMatcherRegex index)
            , if showDeleteButton then
                iconButtonMsg "btn btn-secondary ml-auto" "fa-trash-o" (DeleteMatcher index)

              else
                text ""
            ]
        ]
        |> Html.map UpdateField
